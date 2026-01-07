# Yle scraper
Get RSS feeds from the Finnish Broadcasting Company from any field of interest.
## How to use
Find `config.json` in project root.
* `search_query` is what you'd normally type in the search field
* `search_service` is one of
	- `` (empty string) for no filtering
	- `uutiset` for news
	- `urheilu` for sport news
	- `oppiminen` for learning content
	- `elava-arkisto` for archived (old) contents
	- `ylex` for a station-specific query
* `result-type` is one of
	- `` (empty string) for no filtering
	- `article` for written content
	- `areena` for AV content
* `output_file_path`: defines where the RSS xml will be written. Accepts tilde and absolute paths.
* `useGCS`: whether to upload or not to GCS Bucket, define path below
* `GCSBucket`: path to your GCS Bucket to upload results. 

## Prequisites
### GCP
* You must [set up an authentication method](https://docs.cloud.google.com/docs/authentication/application-default-credentials) for Google Cloud
	- Using Google Cloud [incurs costs](https://cloud.google.com/products/calculator)
* [Set up a Bucket](https://docs.cloud.google.com/storage/docs/creating-buckets) in Cloud Storage
	- Set appropriate view permissions if public exposure is required (may include overriding organization policies)
		- **Only modify these at the project level**, in Google Cloud Console:
			- `constraints.iam.allowedPolicyMemberDomains`
			- `iam.allowedPolicyMemberDomains`

	#### IAM
	- For single-run, `Storage Object Creator` role is sufficient
	- If running recurrently with single query, the service account must have these roles:
		- `Storage Object User`
---
## Example output
See the example [here](https://storage.googleapis.com/yle-scraper-public-demo/xtxmarkets.xml) or an excerpt below:
```xml
<rss version="2.0">
<channel>
<title>Yle Search Results for 'xtx markets'</title>
<link>
https://haku.yle.fi/?page=1&query=%22xtx+markets%22&service=uutiset&type=article
</link>
<description>
Articles from Yle generated via scraping for: xtx markets
</description>
<language>fi</language>
<item>
<title>
XTX voi työllistää Kajaanin data­keskukseensa 50 henkilöä – ensimmäisiä tehtäviä täytetään nyt
</title>
<link>https://yle.fi/a/74-20184002</link>
<pubDate>Tue, 23 Sep 2025 00:00:00 +0300</pubDate>
<guid>https://yle.fi/a/74-20184002</guid>
</item>
...
```