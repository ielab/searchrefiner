# Plugins

Plugins are a way to extend the searchrefiner interface. Plugins allow for tight integration with searchrefiner, including authentication, low-level API access (e.g., loaded known-relevant PMIDs), and access to configuration items.

## Developing a plugin

A plugin is essentially an implementation of the `Plugin` Go interface:

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

To run searchrefiner, building all plugins, use `make run`.
