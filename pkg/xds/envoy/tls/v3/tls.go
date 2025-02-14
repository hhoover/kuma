package v3

import (
	envoy_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_tls "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_type_matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"

	"github.com/kumahq/kuma/pkg/tls"
	util_proto "github.com/kumahq/kuma/pkg/util/proto"
	xds_context "github.com/kumahq/kuma/pkg/xds/context"
	xds_tls "github.com/kumahq/kuma/pkg/xds/envoy/tls"
)

// CreateDownstreamTlsContext creates DownstreamTlsContext for incoming connections
// It verifies that incoming connection has TLS certificate signed by Mesh CA with URI SAN of prefix spiffe://{mesh_name}/
// It secures inbound listener with certificate of "identity_cert" that will be received from the SDS (it contains URI SANs of all inbounds).
func CreateDownstreamTlsContext(ctx xds_context.Context) (*envoy_tls.DownstreamTlsContext, error) {
	if !ctx.Mesh.Resource.MTLSEnabled() {
		return nil, nil
	}
	validationSANMatcher := MeshSpiffeIDPrefixMatcher(ctx.Mesh.Resource.Meta.GetName())
	commonTlsContext, err := createCommonTlsContext(validationSANMatcher)
	if err != nil {
		return nil, err
	}
	return &envoy_tls.DownstreamTlsContext{
		CommonTlsContext:         commonTlsContext,
		RequireClientCertificate: util_proto.Bool(true),
	}, nil
}

// CreateUpstreamTlsContext creates UpstreamTlsContext for outgoing connections
// It verifies that the upstream server has TLS certificate signed by Mesh CA with URI SAN of spiffe://{mesh_name}/{upstream_service}
// The downstream client exposes for the upstream server cert with multiple URI SANs, which means that if DP has inbound with services "web" and "web-api" and communicates with "backend"
// the upstream server ("backend") will see that DP with TLS certificate of URIs of "web" and "web-api".
// There is no way to correlate incoming request to "web" or "web-api" with outgoing request to "backend" to expose only one URI SAN.
//
// Pass "*" for upstreamService to validate that upstream service is a service that is part of the mesh (but not specific one)
func CreateUpstreamTlsContext(ctx xds_context.Context, upstreamService string, sni string) (*envoy_tls.UpstreamTlsContext, error) {
	if !ctx.Mesh.Resource.MTLSEnabled() {
		return nil, nil
	}
	var validationSANMatcher *envoy_type_matcher.StringMatcher
	if upstreamService == "*" {
		validationSANMatcher = MeshSpiffeIDPrefixMatcher(ctx.Mesh.Resource.Meta.GetName())
	} else {
		validationSANMatcher = ServiceSpiffeIDMatcher(ctx.Mesh.Resource.Meta.GetName(), upstreamService)
	}
	commonTlsContext, err := createCommonTlsContext(validationSANMatcher)
	if err != nil {
		return nil, err
	}
	commonTlsContext.AlpnProtocols = xds_tls.KumaALPNProtocols
	return &envoy_tls.UpstreamTlsContext{
		CommonTlsContext: commonTlsContext,
		Sni:              sni,
	}, nil
}

func createCommonTlsContext(validationSANMatcher *envoy_type_matcher.StringMatcher) (*envoy_tls.CommonTlsContext, error) {
	meshCaSecret := sdsSecretConfig(xds_tls.MeshCaResource)
	identitySecret := sdsSecretConfig(xds_tls.IdentityCertResource)
	return &envoy_tls.CommonTlsContext{
		ValidationContextType: &envoy_tls.CommonTlsContext_CombinedValidationContext{
			CombinedValidationContext: &envoy_tls.CommonTlsContext_CombinedCertificateValidationContext{
				DefaultValidationContext: &envoy_tls.CertificateValidationContext{
					MatchSubjectAltNames: []*envoy_type_matcher.StringMatcher{validationSANMatcher},
				},
				ValidationContextSdsSecretConfig: meshCaSecret,
			},
		},
		TlsCertificateSdsSecretConfigs: []*envoy_tls.SdsSecretConfig{
			identitySecret,
		},
	}, nil
}

func sdsSecretConfig(name string) *envoy_tls.SdsSecretConfig {
	return &envoy_tls.SdsSecretConfig{
		Name: name,
		SdsConfig: &envoy_core.ConfigSource{
			ResourceApiVersion:    envoy_core.ApiVersion_V3,
			ConfigSourceSpecifier: &envoy_core.ConfigSource_Ads{},
		},
	}
}

func UpstreamTlsContextOutsideMesh(ca, cert, key []byte, allowRenegotiation bool, hostname string, sni string) (*envoy_tls.UpstreamTlsContext, error) {
	var tlsCertificates []*envoy_tls.TlsCertificate
	if cert != nil && key != nil {
		tlsCertificates = []*envoy_tls.TlsCertificate{
			{
				CertificateChain: dataSourceFromBytes(cert),
				PrivateKey:       dataSourceFromBytes(key),
			},
		}
	}

	var validationContextType *envoy_tls.CommonTlsContext_ValidationContext
	if ca != nil {
		validationContextType = &envoy_tls.CommonTlsContext_ValidationContext{
			ValidationContext: &envoy_tls.CertificateValidationContext{
				TrustedCa: dataSourceFromBytes(ca),
				MatchSubjectAltNames: []*envoy_type_matcher.StringMatcher{
					{
						MatchPattern: &envoy_type_matcher.StringMatcher_Exact{
							Exact: hostname,
						},
					},
				},
			},
		}
	}

	return &envoy_tls.UpstreamTlsContext{
		AllowRenegotiation: allowRenegotiation,
		Sni:                sni,
		CommonTlsContext: &envoy_tls.CommonTlsContext{
			TlsCertificates:       tlsCertificates,
			ValidationContextType: validationContextType,
		},
	}, nil
}

func dataSourceFromBytes(bytes []byte) *envoy_core.DataSource {
	return &envoy_core.DataSource{
		Specifier: &envoy_core.DataSource_InlineBytes{
			InlineBytes: bytes,
		},
	}
}

func MeshSpiffeIDPrefixMatcher(mesh string) *envoy_type_matcher.StringMatcher {
	return &envoy_type_matcher.StringMatcher{
		MatchPattern: &envoy_type_matcher.StringMatcher_Prefix{
			Prefix: xds_tls.MeshSpiffeIDPrefix(mesh),
		},
	}
}

func ServiceSpiffeIDMatcher(mesh string, service string) *envoy_type_matcher.StringMatcher {
	return &envoy_type_matcher.StringMatcher{
		MatchPattern: &envoy_type_matcher.StringMatcher_Exact{
			Exact: xds_tls.ServiceSpiffeID(mesh, service),
		},
	}
}

func KumaIDMatcher(tagName, tagValue string) *envoy_type_matcher.StringMatcher {
	return &envoy_type_matcher.StringMatcher{
		MatchPattern: &envoy_type_matcher.StringMatcher_Exact{
			Exact: xds_tls.KumaID(tagName, tagValue),
		},
	}
}

func StaticDownstreamTlsContext(keyPair *tls.KeyPair) *envoy_tls.DownstreamTlsContext {
	return &envoy_tls.DownstreamTlsContext{
		CommonTlsContext: &envoy_tls.CommonTlsContext{
			TlsCertificates: []*envoy_tls.TlsCertificate{
				{
					CertificateChain: &envoy_core.DataSource{
						Specifier: &envoy_core.DataSource_InlineBytes{
							InlineBytes: keyPair.CertPEM,
						},
					},
					PrivateKey: &envoy_core.DataSource{
						Specifier: &envoy_core.DataSource_InlineBytes{
							InlineBytes: keyPair.KeyPEM,
						},
					},
				},
			},
		},
	}
}
