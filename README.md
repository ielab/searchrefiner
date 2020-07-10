# searchrefiner

_Systematic Review Query Visualisation and Understanding Interface_

searchrefiner is an interactive interface for visualising and understanding queries used to retrieve medical literature for
[systematic reviews](https://en.wikipedia.org/wiki/Systematic_review).

It is currently in development, however please find a demo link [on the project home page](https://ielab.io/searchrefiner).

## Documentation

Documentation for authentication, administration, and usage can be found at the project homepage: 
[ielab.io/searchrefiner](https://ielab.io/searchrefiner)

## Building

searchrefiner is built as a Go application. It can be installed via:

```bash
go install github.com/ielab/searchrefiner
```

The application can then be configured via a `config.json` (a [sample](sample.config.json) is provided). Many of the tools require specific attributes in the configuration. Please get in contact if you are setting up your own instance of searchrefiner to determine how these advances configuration items should be set.

## Citing

Please cite any references to the searchrefiner project as:

```
@inproceedings{scells2018searchrefiner,
    Author = {Scells, Harrisen and Zuccon, Guido},
    Booktitle = {Proceedings of the 27th ACM International Conference on Information and Knowledge Management},
    Organization = {ACM},
    Title = {searchrefiner: A Query Visualisation and Understanding Tool for Systematic Reviews},
    Year = {2018}
}
```

Please cite any references to any of the automation tools embedded in searchrefiner as:

```
@inproceedings{li2020systematic,
	Author = {Li, Hang and Scells, Harrisen and Zuccon, Guido},
	Booktitle = {Proceedings of the 43rd Internationa SIGIR Conference on Research and Development in Information Retrieval},
	Date-Added = {2020-06-09 13:11:19 +1000},
	Date-Modified = {2020-07-03 15:45:14 +1000},
	Month = {July},
	Pages = {25--30},
	Title = {Systematic Review Automation Tools for End-to-End Query Formulation},
	Year = {2020}
}
```
