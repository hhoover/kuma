//go:build gateway
// +build gateway

package deployments

import "embed"

//go:embed charts/kuma/crds/gateway/*
var gatewayAPICRDsData embed.FS

func init() {
	additionalData = append(additionalData, gatewayAPICRDsData)
}
