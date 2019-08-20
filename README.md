# searchrefiner

_Systematic Review Query Visualisation and Understanding Interface_

![home](_docs/assets/images/home.png)

searchrefiner is an interactive interface for visualising and understanding queries used to retrieve medical literature for
[systematic reviews](https://en.wikipedia.org/wiki/Systematic_review).

It is currently in development, however you may preview the interface at this [demo link](http://43.240.96.223:4853/).

## Documentation

Documentation for authentication, administration, and usage can be found at the project homepage: 
[ielab.io/searchrefiner](https://ielab.io/searchrefiner)

## Building

searchrefiner is built as a Go application. It can be installed via:

```bash
go install github.com/ielab/searchrefiner
```

The application can then be configured via `config.json`. The configuration items should be self explanatory.

## Citing

If you use this work, please cite as:

```
@inproceedings{scells2018searchrefiner,
    Author = {Scells, Harrisen and Zuccon, Guido},
    Booktitle = {Proceedings of the 27th ACM International Conference on Information and Knowledge Management},
    Organization = {ACM},
    Title = {searchrefiner: A Query Visualisation and Understanding Tool for Systematic Reviews},
    Year = {2018}
}
```