package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bitnixdev/terraform-provider-skipjack/internal/client"
)

var (
	_ datasource.DataSource              = &VariablesDataSource{}
	_ datasource.DataSourceWithConfigure = &VariablesDataSource{}
)

// VariablesDataSource lists all variables in a scope.
type VariablesDataSource struct {
	client *client.Client
}

type variablesDataSourceModel struct {
	ID      types.String `tfsdk:"id"`
	Org     types.String `tfsdk:"org"`
	Project types.String `tfsdk:"project"`
	Names   types.List   `tfsdk:"names"`
	Values  types.Map    `tfsdk:"values"`
}

func NewVariablesDataSource() datasource.DataSource {
	return &VariablesDataSource{}
}

func (d *VariablesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_variables"
}

func (d *VariablesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists all non-secret variables in an org or project scope. " +
			"Requires a token API key with `variablesRead`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Scope identifier: `org/project` or `org/`.",
			},
			"org": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Skipjack organization slug.",
			},
			"project": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Project slug. Omit for org-level variables.",
			},
			"names": schema.ListAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				MarkdownDescription: "Sorted list of variable names in the scope.",
			},
			"values": schema.MapAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				MarkdownDescription: "Map of variable name to value.",
			},
		},
	}
}

func (d *VariablesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *VariablesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data variablesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scope, diags := scopeFromAttrs(data.Org, data.Project)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	variables, err := d.client.ListVariables(ctx, scope)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list variables", err.Error())
		return
	}

	sortVariablesByName(variables)

	nameAttrs := make([]attr.Value, 0, len(variables))
	valueAttrs := make(map[string]attr.Value, len(variables))
	for _, v := range variables {
		nameAttrs = append(nameAttrs, types.StringValue(v.Name))
		valueAttrs[v.Name] = types.StringValue(v.Value)
	}

	names, listDiags := types.ListValue(types.StringType, nameAttrs)
	resp.Diagnostics.Append(listDiags...)
	values, mapDiags := types.MapValue(types.StringType, valueAttrs)
	resp.Diagnostics.Append(mapDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.ID = types.StringValue(fmt.Sprintf("%s/%s", scope.Org, scope.Project))
	data.Names = names
	data.Values = values

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func sortVariablesByName(variables []client.Variable) {
	sort.Slice(variables, func(i, j int) bool {
		return variables[i].Name < variables[j].Name
	})
}
