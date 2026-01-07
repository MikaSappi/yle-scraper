# Yle scraper
## How to use
Find `config.json` in project root.
* `search_query` is what you'd normally type in the search field
* `search_service` is one of
	- `uutiset` for news
	- `urheilu` for sport news
	- `oppiminen` for learning content
	- `elava-arkisto` for archived (old) contents
	- `ylex` for a station-specific query
* `result-type` is one of
	- `` (empty string) for all no filtering
	- `article` for written content
	- `areena` for AV content