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
	_ resource.Resource                = &SecretResource{}
	_ resource.ResourceWithConfigure   = &SecretResource{}
	_ resource.ResourceWithImportState = &SecretResource{}
)

// SecretResource manages a Skipjack secret via PUT/GET/DELETE on /v1.
// Analogous to vault_kv_secret_v2 / vault_generic_secret for a single named value.
type SecretResource struct {
	client *client.Client
}

type secretResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Org         types.String `tfsdk:"org"`
	Project     types.String `tfsdk:"project"`
	Name        types.String `tfsdk:"name"`
	Value       types.String `tfsdk:"value"`
	Description types.String `tfsdk:"description"`
	Version     types.Int64  `tfsdk:"version"`
}

func NewSecretResource() resource.Resource {
	return &SecretResource{}
}

func (r *SecretResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_secret"
}

func (r *SecretResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a single secret in Skipjack (org-level or project-level). " +
			"Creates and updates use the token API `PUT /v1/orgs/.../secrets/:name`, which versions " +
			"the secret value server-side. Comparable to `vault_kv_secret_v2` for a single key/value.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Stable identifier: `org/project/name`, or `org//name` for org-level secrets.",
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
				MarkdownDescription: "Project slug. Omit (or leave null) for an org-level secret. Changing project forces replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Secret name. Must be a valid environment variable name (`^[A-Za-z_][A-Za-z0-9_]*$`). Changing name forces replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"value": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "Secret plaintext value. Stored in Terraform state (sensitive).",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional human-readable description (max 512 characters).",
			},
			"version": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Server-side secret version after the last write.",
			},
		},
	}
}

func (r *SecretResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data secretResourceModel
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

	result, err := r.client.PutSecret(
		ctx,
		scope,
		data.Name.ValueString(),
		data.Value.ValueString(),
		optionalStringPtr(data.Description),
	)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create secret", err.Error())
		return
	}

	data.ID = types.StringValue(client.ResourceID(scope.Org, scope.Project, data.Name.ValueString()))
	data.Version = types.Int64Value(result.Version)

	tflog.Trace(ctx, "created skipjack_secret", map[string]any{
		"id":      data.ID.ValueString(),
		"version": result.Version,
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data secretResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scope, diags := scopeFromAttrs(data.Org, data.Project)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	secret, err := r.client.GetSecret(ctx, scope, data.Name.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read secret", err.Error())
		return
	}

	data.Value = types.StringValue(secret.Value)
	data.Version = types.Int64Value(secret.Version)
	data.ID = types.StringValue(client.ResourceID(scope.Org, scope.Project, secret.Name))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data secretResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scope, diags := scopeFromAttrs(data.Org, data.Project)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.PutSecret(
		ctx,
		scope,
		data.Name.ValueString(),
		data.Value.ValueString(),
		optionalStringPtr(data.Description),
	)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update secret", err.Error())
		return
	}

	data.ID = types.StringValue(client.ResourceID(scope.Org, scope.Project, data.Name.ValueString()))
	data.Version = types.Int64Value(result.Version)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data secretResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scope, diags := scopeFromAttrs(data.Org, data.Project)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteSecret(ctx, scope, data.Name.ValueString()); err != nil {
		if client.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Unable to delete secret", err.Error())
		return
	}
}

func (r *SecretResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
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
