package provider

import (
	"context"
	"os"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/bitnixdev/terraform-provider-skipjack/internal/client"
)

// Ensure SkipjackProvider satisfies provider.Provider.
var _ provider.Provider = &SkipjackProvider{}

// SkipjackProvider implements the Terraform provider.
type SkipjackProvider struct {
	// version is "dev" locally, "test" in acceptance tests, or a release tag.
	version string
}

// SkipjackProviderModel is the provider configuration schema.
type SkipjackProviderModel struct {
	URL          types.String `tfsdk:"url"`
	Token        types.String `tfsdk:"token"`
	OIDCAudience types.String `tfsdk:"oidc_audience"`
}

func (p *SkipjackProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "skipjack"
	resp.Version = p.version
}

func (p *SkipjackProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The Skipjack provider manages secrets and variables through Skipjack's " +
			"machine-facing token API (`/v1`). Authentication is either a long-lived API key (`sjk_...`) " +
			"or a short-lived OIDC JWT for a configured **workload identity** (GitHub Actions, Terraform Cloud, etc.). " +
			"This is a focused subset of the HashiCorp Vault provider's secret surface: " +
			"read/write individual secrets and variables, and list them within an org or project scope.",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				MarkdownDescription: "HTTPS base URL of the Skipjack deployment (origin only). " +
					"Defaults to `https://skipjack.bitnix.dev`. " +
					"May also be set via the `SKIPJACK_URL` environment variable.",
				Optional: true,
			},
			"token": schema.StringAttribute{
				MarkdownDescription: "Bearer credential for the token API: either a Skipjack API key (`sjk_...`) " +
					"or an OIDC JWT that matches a configured workload identity. " +
					"May also be set via the `SKIPJACK_TOKEN` environment variable. " +
					"When unset in GitHub Actions (with `permissions: id-token: write`), the provider " +
					"automatically requests an OIDC id-token using `oidc_audience`.",
				Optional:  true,
				Sensitive: true,
			},
			"oidc_audience": schema.StringAttribute{
				MarkdownDescription: "Audience claim used when minting a GitHub Actions OIDC token " +
					"(and documented for other CI systems that inject JWTs). " +
					"Defaults to the provider `url`. " +
					"Must match the audience configured on the Skipjack OIDC workload identity. " +
					"May also be set via the `SKIPJACK_OIDC_AUDIENCE` environment variable.",
				Optional: true,
			},
		},
	}
}

func (p *SkipjackProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config SkipjackProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Resolution: provider attribute → SKIPJACK_URL → https://skipjack.bitnix.dev
	url := ""
	if !config.URL.IsNull() && !config.URL.IsUnknown() {
		url = strings.TrimSpace(config.URL.ValueString())
	}
	if url == "" {
		url = strings.TrimSpace(os.Getenv("SKIPJACK_URL"))
	}
	if url == "" {
		url = client.DefaultURL
	}
	url = strings.TrimRight(url, "/")
	if url == "" {
		url = client.DefaultURL
	}

	token := ""
	if !config.Token.IsNull() && !config.Token.IsUnknown() {
		token = strings.TrimSpace(config.Token.ValueString())
	}
	if token == "" {
		token = strings.TrimSpace(os.Getenv("SKIPJACK_TOKEN"))
	}

	audience := url
	if !config.OIDCAudience.IsNull() && !config.OIDCAudience.IsUnknown() && config.OIDCAudience.ValueString() != "" {
		audience = config.OIDCAudience.ValueString()
	} else if v := os.Getenv("SKIPJACK_OIDC_AUDIENCE"); v != "" {
		audience = v
	}

	// When no explicit credential is provided, try GitHub Actions OIDC so CI
	// can use workload identities without shipping a long-lived API key.
	if token == "" && client.GitHubActionsOIDCAvailable() {
		tflog.Info(ctx, "requesting GitHub Actions OIDC token for Skipjack", map[string]any{
			"audience": audience,
		})
		jwt, err := client.FetchGitHubActionsOIDCToken(ctx, audience)
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to obtain GitHub Actions OIDC token",
				err.Error()+"\n\nEnsure the job has `permissions: id-token: write` and that "+
					"a Skipjack OIDC workload identity is configured for this repository's subject "+
					"with a matching audience.",
			)
			return
		}
		token = jwt
		tflog.Info(ctx, "using GitHub Actions OIDC JWT for Skipjack /v1 auth")
	}

	if token == "" {
		resp.Diagnostics.AddError(
			"Missing Skipjack credentials",
			"The provider requires a credential for the /v1 token API.\n\n"+
				"Set one of:\n"+
				"  • token / SKIPJACK_TOKEN — API key (sjk_...) or OIDC JWT\n"+
				"  • Run in GitHub Actions with id-token: write (auto OIDC fetch)\n\n"+
				"OIDC JWTs must match a configured Skipjack workload identity "+
				"(issuer, audience, subject pattern, scopes, and permissions).",
		)
		return
	}

	if client.IsAPIKey(token) {
		tflog.Debug(ctx, "authenticating to Skipjack with API key")
	} else if client.IsJWT(token) {
		tflog.Debug(ctx, "authenticating to Skipjack with OIDC JWT")
	} else {
		tflog.Warn(ctx, "Skipjack token does not look like an API key or JWT; sending as Bearer anyway")
	}

	c, err := client.New(url, token, "terraform-provider-skipjack/"+p.version)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create Skipjack client",
			err.Error(),
		)
		return
	}

	resp.DataSourceData = c
	resp.ResourceData = c
}

func (p *SkipjackProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewSecretResource,
		NewVariableResource,
	}
}

func (p *SkipjackProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewSecretDataSource,
		NewSecretsDataSource,
		NewVariableDataSource,
		NewVariablesDataSource,
	}
}

// New returns a provider factory.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &SkipjackProvider{version: version}
	}
}
