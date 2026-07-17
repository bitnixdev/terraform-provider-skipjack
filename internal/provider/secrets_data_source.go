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
	_ datasource.DataSource              = &SecretsDataSource{}
	_ datasource.DataSourceWithConfigure = &SecretsDataSource{}
)

// SecretsDataSource lists all secrets in a scope (values + names).
// Combines vault_kv_secrets_list (names) and vault_generic_secret (map of data).
type SecretsDataSource struct {
	client *client.Client
}

type secretsDataSourceModel struct {
	ID      types.String `tfsdk:"id"`
	Org     types.String `tfsdk:"org"`
	Project types.String `tfsdk:"project"`
	Names   types.List   `tfsdk:"names"`
	Values  types.Map    `tfsdk:"values"`
}

func NewSecretsDataSource() datasource.DataSource {
	return &SecretsDataSource{}
}

func (d *SecretsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_secrets"
}

func (d *SecretsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists all secrets in an org or project scope and returns their values. " +
			"Requires a token API key with `secretsRead`. " +
			"Similar to combining Vault's list and read data sources for a mount path.",
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
				MarkdownDescription: "Project slug. Omit for org-level secrets.",
			},
			"names": schema.ListAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				MarkdownDescription: "Sorted list of secret names in the scope.",
			},
			"values": schema.MapAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Map of secret name to decrypted value.",
			},
		},
	}
}

func (d *SecretsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SecretsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data secretsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scope, diags := scopeFromAttrs(data.Org, data.Project)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	secrets, err := d.client.ListSecrets(ctx, scope)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list secrets", err.Error())
		return
	}

	// Stable sort by name for deterministic plans.
	sortSecretsByName(secrets)

	nameAttrs := make([]attr.Value, 0, len(secrets))
	valueAttrs := make(map[string]attr.Value, len(secrets))
	for _, s := range secrets {
		nameAttrs = append(nameAttrs, types.StringValue(s.Name))
		valueAttrs[s.Name] = types.StringValue(s.Value)
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

func sortSecretsByName(secrets []client.Secret) {
	sort.Slice(secrets, func(i, j int) bool {
		return secrets[i].Name < secrets[j].Name
	})
}
