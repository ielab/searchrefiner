# searchrefiner API

searchrefiner, once logged in, exposes the following application programming interfaces. These can be used to integrate or 
augment searchrefiner into other tools.

For information on authenticating to searchrefiner, see the article on [authentication](authentication.md).

## Authentication APIs

Before using the general APIs, a user must be authenticated. The authentication APIs are listed as follows:

 - `/account/api/create`: For creating an account. The parameters of this endpoint are `username`, `password`, and 
 `password2`. For an account to be created, the username must not exist, and the two password parameters must match.
 - `/account/api/login`: For logging into an account. The parameters of this endpoint are `username`, `password`. This
 endpoint will set an authentication cookie if the username and password match.
 - `/account/api/logout`: For logging out of an account. A cookie token must be passed to the endpoint. This endpoint
 will unset the cookie and revoke the authentication token server side.
 
## searchrefiner APIs

Once authenticated, the following APIs can be used:

 - `/api/query2cqr`: For transforming a query into the [common query representation](https://github.com/hscells/cqr).
 This representation is similar to an abstract syntax tree. The parameters of this endpoint are `query` and `lang`.
 The `query` parameter is a query and the `lang` parameter is the language of the `query` parameter ("pubmed" or 
 "medline").
 - `/api/cqr2query`: For transforming a common query representation into a query. The parameters of this endpoint are 
 `query` and `lang`. The `query` parameter is a common query representation and the `lang` parameter is the language of 
 the `query` to transform it into ("pubmed" or "medline").
 - `api/tree`: For building a tree representation of a query. The parameters of this endpoint are `query` and `lang`.
 The `query` parameter is a query and the `lang` parameter is the language of the `query` parameter ("pubmed" or 
 "medline"). The response of this endpoint is a list of nodes and edges in the [vis.js](http://visjs.org/docs/network/)
 format.
 
## Further Links

 - [Home](index.md)
 - [Set-up](setup.md)
 - [Authentication](authentication.md)
 - [Tools](tools.md)