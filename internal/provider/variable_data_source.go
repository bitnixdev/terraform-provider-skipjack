package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bitnixdev/terraform-provider-skipjack/internal/client"
)

var (
	_ datasource.DataSource              = &VariableDataSource{}
	_ datasource.DataSourceWithConfigure = &VariableDataSource{}
)

// VariableDataSource reads one non-secret variable.
type VariableDataSource struct {
	client *client.Client
}

type variableDataSourceModel struct {
	ID      types.String `tfsdk:"id"`
	Org     types.String `tfsdk:"org"`
	Project types.String `tfsdk:"project"`
	Name    types.String `tfsdk:"name"`
	Value   types.String `tfsdk:"value"`
}

func NewVariableDataSource() datasource.DataSource {
	return &VariableDataSource{}
}

func (d *VariableDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_variable"
}

func (d *VariableDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single non-secret variable from Skipjack by name. " +
			"Requires a token API key with `variablesRead`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Stable identifier: `org/project/name` or `org//name`.",
			},
			"org": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Skipjack organization slug.",
			},
			"project": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Project slug. Omit for an org-level variable.",
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Variable name.",
			},
			"value": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Variable value.",
			},
		},
	}
}

func (d *VariableDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData),
		)
		return
	}
	d.client = c
}

func (d *VariableDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data variableDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scope, diags := scopeFromAttrs(data.Org, data.Project)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	variable, err := d.client.GetVariable(ctx, scope, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read variable", err.Error())
		return
	}

	data.ID = types.StringValue(client.ResourceID(scope.Org, scope.Project, variable.Name))
	data.Value = types.StringValue(variable.Value)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
