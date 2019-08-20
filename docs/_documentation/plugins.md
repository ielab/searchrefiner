---
title: "Plugins"
weight: 2
---

Plugins are a way to extend the searchrefiner interface. Plugins allow for tight integration with searchrefiner, including authentication, low-level API access (e.g., loaded known-relevant PMIDs), and access to configuration items.

## Developing a plugin

A plugin is an implementation of the searchrefiner Go Plugin interface:

```go
type Plugin interface {
	Serve(Server, *gin.Context)
	PermissionType() PluginPermission
}
```

For the searchrefiner server to register a HTTP handler to a plugin the following requirements must be met (we will use `example` as the name of our plugin):

 1. The plugin must reside in the `/plugin/example` path.
 2. The package of the main go file must be `main`.
 3. The package must export a variable (called `Example`) that implements the `Plugin` interface.

searchrefiner will then create a route of the same name of the plugin.

An example file containing this implementation is below:

```go
package main

import (
	"github.com/gin-gonic/gin"
	"github.com/ielab/searchrefiner"
	"net/http"
)

// example is the concrete implementation for the plugin.
type example struct {	
}

func (e example) Serve(s searchrefiner.Server, c *gin.Context) {
    c.String(http.StatusOK, "it works!")
    return
}

func (e example) PermissionType() searchrefiner.PluginPermission {
    return searchrefiner.PluginUser
}

var Example example // Example is the exported variable.
```

The make system will build and include all plugins in the `plugin` path automatically.