# Authentication

Users must be authenticated to CiteMed before using the tools and APIs. Configuration of administrators can be done via
editing the configuration items in `config.json`, as explained in [setup instructions](setup.md).

For command-line and programmatic API usage, a cookie authentication token must be acquired. An example of this token 
acquisition using curl can be performed as follows:

```bash
curl -X POST -v citemed.url/account/api/login -F 'username=example' -F 'password=12345'
```

The authentication cookie token will be set in the response header if the username and password are correct:

```
...
< Set-Cookie: user=sdfdfg5325==|53462342362|48798fg8229991288fhfnaasd3819t51; <<truncated>>
...
```

_(obviously fake cookie)_

Now when using the API, requests can be made like so:

```bash
curl -X POST --cookie "user=sdfdfg5325==|53462342362|48798fg8229991288fhfnaasd3819t51" localhost:4853/api/query2cqr -F 'query=(neck[Title] AND cancer[Abstract])' -F 'lang=pubmed'
```

## Further Links

 - [Home](index.md)
 - [Set-up](setup.md)
 - [API](api.md)
 - [Tools](tools.md)