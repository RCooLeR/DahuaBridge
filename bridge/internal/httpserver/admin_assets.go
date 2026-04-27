package httpserver

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed assets/*
var adminAssetFiles embed.FS

var embeddedAdminAssets = mustEmbeddedAdminAssets()

func mustEmbeddedAdminAssets() fs.FS {
	subtree, err := fs.Sub(adminAssetFiles, "assets")
	if err != nil {
		panic(err)
	}
	return subtree
}

func adminAssetHandler() http.Handler {
	return http.StripPrefix("/admin/assets/", http.FileServer(http.FS(embeddedAdminAssets)))
}
