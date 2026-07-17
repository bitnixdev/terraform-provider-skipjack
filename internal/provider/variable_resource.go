package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/bitnixdev/terraform-provider-skipjack/internal/client"
)

var (
	_ resource.Resource                = &VariableResource{}
	_ resource.ResourceWithConfigure   = &VariableResource{}
	_ resource.ResourceWithImportState = &VariableResource{}
)

// VariableResource manages a Skipjack non-secret variable via the token API.
type VariableResource struct {
	client *client.Client
}

type variableResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Org         types.String `tfsdk:"org"`
	Project     types.String `tfsdk:"project"`
	Name        types.String `tfsdk:"name"`
	Value       types.String `tfsdk:"value"`
	Description types.String `tfsdk:"description"`
}

func NewVariableResource() resource.Resource {
	return &VariableResource{}
}

func (r *VariableResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_variable"
}

func (r *VariableResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a single non-secret variable in Skipjack (org-level or project-level). " +
			"Variables are stored in plaintext server-side and are not masked when exported to GitHub Actions. " +
			"Use `skipjack_secret` for sensitive values.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Stable identifier: `org/project/name`, or `org//name` for org-level variables.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"org": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Skipjack organization slug.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"project": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Project slug. Omit (or leave null) for an org-level variable. Changing project forces replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Variable name. Must be a valid environment variable name (`^[A-Za-z_][A-Za-z0-9_]*$`). Changing name forces replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"value": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Variable value (non-secret configuration).",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional human-readable description (max 512 characters).",
			},
		},
	}
}

func (r *VariableResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData),
		)
		return
	}
	r.client = c
}

func (r *VariableResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data variableResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := validateName(data.Name.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("name"), "Invalid name", err.Error())
		return
	}

	scope, diags := scopeFromAttrs(data.Org, data.Project)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.PutVariable(
		ctx,
		scope,
		data.Name.ValueString(),
		data.Value.ValueString(),
		optionalStringPtr(data.Description),
	); err != nil {
		resp.Diagnostics.AddError("Unable to create variable", err.Error())
		return
	}

	data.ID = types.StringValue(client.ResourceID(scope.Org, scope.Project, data.Name.ValueString()))

	tflog.Trace(ctx, "created skipjack_variable", map[string]any{
		"id": data.ID.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VariableResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data variableResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scope, diags := scopeFromAttrs(data.Org, data.Project)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	variable, err := r.client.GetVariable(ctx, scope, data.Name.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read variable", err.Error())
		return
	}

	data.Value = types.StringValue(variable.Value)
	data.ID = types.StringValue(client.ResourceID(scope.Org, scope.Project, variable.Name))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VariableResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data variableResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scope, diags := scopeFromAttrs(data.Org, data.Project)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.PutVariable(
		ctx,
		scope,
		data.Name.ValueString(),
		data.Value.ValueString(),
		optionalStringPtr(data.Description),
	); err != nil {
		resp.Diagnostics.AddError("Unable to update variable", err.Error())
		return
	}

	data.ID = types.StringValue(client.ResourceID(scope.Org, scope.Project, data.Name.ValueString()))
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VariableResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data variableResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scope, diags := scopeFromAttrs(data.Org, data.Project)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteVariable(ctx, scope, data.Name.ValueString()); err != nil {
		if client.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Unable to delete variable", err.Error())
		return
	}
}

func (r *VariableResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	org, project, name, err := client.ParseResourceID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import id", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("org"), org)...)
	if project == "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project"), types.StringNull())...)
	} else {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project"), project)...)
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
}
