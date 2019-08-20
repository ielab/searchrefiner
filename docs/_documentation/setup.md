---
title: "Setup"
weight: 1
---

searchrefiner is built as a Go application. To build the application from source, the [Go toolchain](https://golang.org/) is 
required. Once installed, the software may be installed by cloning:

```bash
git clone git@github.com:ielab/searchrefiner.git
```

or by directly [downloading](https://github.com/ielab/searchrefiner/archive/master.zip).

searchrefiner is built using `make`. To build and run the application use the command `make run`.

## Configuration

searchrefiner is configured via a configuration file named `config.json`. Upon startup, the software will look for this file.
The configuration items are used as follows:

 - `Host`: The URL and port to run searchrefiner.
 - `AdminEmail`: The administrator who should be contacted for any problems with the software.
 - `Admins`: A list of usernames that are able to access the `/admin` endpoint to approve new users. Note that this list
 must be configured prior to accounts being added (as it is checked before a new user is added).
 - `Entrez.Email`: The email to report to eutils. 
 - `Entrez.APIKey`: The API key to report to eutils. 
  
An example configuration file is presented below:

```json
{
  "Host": "localhost:4853",
  "AdminEmail": "ADMIN_EMAIL_ADDRESS",
  "Admins": [
    "ADMIN_USERNAME"
  ],
  "Entrez": {
    "Email": "YOUR_EMAIL_ADDRESS",
    "APIKey": "YOUR_API_KEY"
  }
}
```

The Entrez options must be entered, and an API key for eutils can be obtained by logging into NCBI and navigating to [https://www.ncbi.nlm.nih.gov/account/settings/](https://www.ncbi.nlm.nih.gov/account/settings/).
  
## Databases

searchrefiner uses BoltDB for storing user information. This file will be created when the software is run for the first time
in the same directory.

## Further Links

This should be enough to get an instance of searchrefiner up and running. For information on using searchrefiner, see the links in the sidebar to the left.