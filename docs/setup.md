# Set-up

searchrefiner is built as a Go application. To build the application from source, the [Go toolchain](https://golang.org/) is 
required. Once installed, the software may be installed via:

```bash
go get github.com/ielab/searchrefiner
```

To run the software, either use `go build` to create a compiled binary and run `./searchrefiner` or use `go run *.go`.

## Configuration

searchrefiner is configured via a configuration file named `config.json`. Upon startup, the software will look for this file.
The configuration items are used as follows:

 - `Host`: The URL and port to run searchrefiner.
 - `AdminEmail`: The administrator who should be contacted for any problems with the software.
 - `Admins`: A list of usernames that are able to access the `/admin` endpoint to approve new users. Note that this list
 must be configured prior to accounts being added (as it is checked before a new user is added).
 - `Entrez.Email`: The email to report to eutils. 
 - `Entrez.APIKey`: The API key to report to eutils. 
  
## Databases

searchrefiner uses BoltDB for storing user information. This file will be created when the software is run for the first time
in the same directory.

## Further Links

This should be enough to get an instance of searchrefiner up and running. For information on using searchrefiner, see the links below:

 - [Home](index.md)
 - [Authentication](authentication.md)
 - [API](api.md)
 - [Tools](tools.md)