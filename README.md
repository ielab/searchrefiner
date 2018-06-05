# CiteMed

_Systematic Review Query Visualisation and Understanding Interface_

![home](docs/assets/images/home.png)

CiteMed is an interactive interface for visualising and understanding queries used to retrieve medical literature for
[systematic reviews](https://en.wikipedia.org/wiki/Systematic_review).

It is currently in development, however you may preview the interface at this [demo link](http://43.240.96.223:4853/).

## Documentation

Documentation for authentication, administration, and usage can be found at the project homepage: 
[ielab.io/citemed](https://ielab.io/citemed)

## Building

CiteMed is built as a Go application. It can be installed via:

```bash
go install github.com/ielab/citemed
```

The application can then be configured via `config.json`. The configuration items should be self explanatory.

### Elasticsearch

Currently, the only way to use CiteMed is with an Elasticsearch index of PubMed. In the near future, a hook into the 
PubMed API will be used for retrieval. For demonstration and development purposes, however, Elasticsearch is currently
the only way to explore the interface.
 
While the index can be specified in the configuration file, CiteMed is looking for the following Elasticsearch mapping:

```json
{
  "pubmed" : {
    "mappings" : {
      "doc" : {
        "properties" : {
          "authors" : {
            "properties" : {
              "first_name" : {
                "type" : "text",
                "fields" : {
                  "keyword" : {
                    "type" : "keyword",
                    "ignore_above" : 256
                  }
                }
              },
              "last_name" : {
                "type" : "text",
                "fields" : {
                  "keyword" : {
                    "type" : "keyword",
                    "ignore_above" : 256
                  }
                }
              }
            }
          },
          "mesh_headings" : {
            "type" : "text",
            "fields" : {
              "keyword" : {
                "type" : "keyword",
                "ignore_above" : 256
              }
            }
          },
          "pubdate" : {
            "type" : "date"
          },
          "publication_types" : {
            "type" : "text",
            "fields" : {
              "keyword" : {
                "type" : "keyword",
                "ignore_above" : 256
              }
            }
          },
          "text" : {
            "type" : "text",
            "fields" : {
              "stemmed" : {
                "type" : "text",
                "analyzer" : "pubmed_analyser"
              }
            },
            "analyzer" : "standard"
          },
          "title" : {
            "type" : "text",
            "fields" : {
              "stemmed" : {
                "type" : "text",
                "analyzer" : "pubmed_analyser"
              }
            },
            "analyzer" : "standard"
          }
        }
      }
    }
  }
}
```