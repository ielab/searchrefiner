# Set-up

CiteMed is built as a Go application. To build the application from source, the [Go toolchain](https://golang.org/) is 
required. Once installed, the software may be installed via:

```bash
go get github.com/ielab/citemed
```

To run the software, either use `go build` to create a compiled binary and run `./citemed` or use `go run *.go`.

## Configuration

CiteMed is configured via a configuration file named `config.json`. Upon startup, the software will look for this file.
The configuration items are used as follows:

 - `AdminEmail`: The administrator who should be contacted for any problems with the software.
 - `Admins`: A list of usernames that are able to access the `/admin` endpoint to approve new users. Note that this list
 must be configured prior to accounts being added (as it is checked before a new user is added).
 - `Elasticsearch`: The URL (containing protocol) of the Elasticsearch instance (refer to the 
 [README](https://github.com/ielab/citemed/blob/master/README.md) for more information on why and how Elasticsearch 
 must be configured).
 - `Index`: The Elasticsearch index to use.
 
**Note**: Future releases will not solely rely on Elasticsearch and in fact will hook into the PubMed retrieval API. 
 
## Databases

CiteMed uses BoltDB for storing user information. This file will be created when the software is run for the first time
in the same directory.

## Further Links

This should be enough to get an instance of CiteMed up and running. For information on using CiteMed, see the links below:

 - [Home](index.md)
 - [Authentication](authentication.md)
 - [API](api.md)
 - [Tools](tools.md)