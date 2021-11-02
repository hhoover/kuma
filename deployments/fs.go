package deployments

import (
	"embed"
	"io/fs"
)

// By default, go embed does not embed files that starts with `.` or `_` that's why we need to add _helpers.tpl explicitly

//go:embed charts/kuma/templates/* charts/kuma/crds/*.yaml charts/kuma/*.yaml
var chartsData embed.FS

var additionalData []embed.FS

func KumaChartFS() fs.FS {
	fsys, err := fs.Sub(chartsData, "charts/kuma")
	if err != nil {
		panic(err)
	}
	return fsys
}

func KumaChartAdditionalFS() []fs.FS {
	var files []fs.FS

	for _, data := range additionalData {
		fsys, err := fs.Sub(data, "charts/kuma")
		if err != nil {
			panic(err)
		}

		files = append(files, fsys)
	}

	return files
}
