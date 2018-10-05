# Changelog

All notable changes to this project will be documented in this file.

## [04.Sep.2018]

### Added

 + Integrate the ability for plugins to be developed and loaded into interface.
 + Create a "chain" plugin based off recent query transformation research (see [plugins](/plugins)).

### Changed

 + Exported several fields and methods for plugin access

## [20.Aug.2018]

### Changed

 + Renamed "CiteMed" to "searchrefiner".
 + Very minor interface tweaks.
 + [[transmute](https://github.com/hscells/transmute)] Improved parsing and compilation of Ovid MEDLINE queries.

## [1.Aug.2018]

### Added

 + Placed a footer on most pages where appropriate to some hopefully handy links.
 + Included query statistics to the query page. 
 + Included statistics about the submitted query on the query page. 
 + [[transmute](https://github.com/hscells/transmute)] Redundant parenthesis are now added automatically where appropriate.
 + [[transmute](https://github.com/hscells/transmute)] Queries will now recognise floating subheading, major mesh terms, journals and a couple of other fields.

### Changed

 + Previous queries and query analysis on query page have been banished to accordions.
 + Various places that mentioned "MEDLINE" now refer to "Ovid MEDLINE".
 + When PMIDs in the settings page are loaded, it is much more obvious that something has happened.

### Removed

 + Overlap test visualisation has been removed from the QueryVis interface.