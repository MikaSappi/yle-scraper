# Yle scraper
Get RSS feeds from the Finnish Broadcasting Company from any field of interest.

## Prequisites
### Google Chrome (Local Development Only)
For local development, this system uses Chrome for web scraping. Install via `brew install google-chrome` or your preferred method.

**Note:** Cloud Run deployment uses Chromium (included in Docker image), so local Chrome installation is not required for production use.
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

## How to use
Find `config.json` in project root. All fields of type string unless otherwise specified. 
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
* `cloudrun`, of type `bool`: Set to `true` for Cloud Run deployment (bypasses local Google Cloud authentication environment checks)
* `useGCS`, of type `bool`: whether to upload or not to GCS Bucket, define path below
* `GCSBucket`: path to your GCS Bucket to upload results.

## Cloud Run
You may want to set this up as a Cloud Run Job. An example `Dockerfile` has been included. Setting this to `true` bypasses environment validation.
### Prepare for Cloud Run
- Set `cloudrun` in `config.json` to `true`
- Prepare a `Dockerfile` or use the provided one
- Prepare a `.gcloudignore` or use the provided one (otherwise `config.json` won't be uploaded to GCP)

#### Set up GCP
Make sure your compute account is privileged to write to your GCS Bucket. The relevant role is `Cloud Storage User`.
```bash
# Set your project ID
PROJECT_ID="your-actual-project-id"
REGION="europe-north1"
gcloud config set project ${PROJECT_ID}

# Enable APIs (one-time setup)
gcloud services enable cloudbuild.googleapis.com
gcloud services enable run.googleapis.com
gcloud services enable cloudscheduler.googleapis.com
gcloud services enable artifactregistry.googleapis.com

# Create Docker repository
gcloud artifacts repositories create yle-scraper-repo \
  --repository-format=docker \
  --location=europe-north1 \
  --description="Yle scraper container images"
  
# Build and push to Artifact Registry
gcloud builds submit --tag $REGION-docker.pkg.dev/$PROJECT_ID/yle-scraper-repo/yle-scraper

# Create Cloud Run Job with new image path
gcloud run jobs create yle-scraper \
  --image $REGION-docker.pkg.dev/$PROJECT_ID/yle-scraper-repo/yle-scraper \
  --region $REGION \
  --max-retries 2 \
  --task-timeout 10m

```

#### Manual run
You may want to test the job before setting up timed runs.
```bash
gcloud run jobs execute yle-scraper --region $REGION --wait

```

#### Cloud Scheduler
Updating the RSS feeds is done by using the Cloud Scheduler. The Cloud Scheduler uses basic Cron syntax. The Cloud Scheduler may run in different Region than where your Job runs in.

In Europe, Cloud Scheduler is available in these Regions:
- europe-central2 (Warsaw)
- europe-west1 (Belgium)
- europe-west2 (London)
- europe-west3 (Frankfurt)
- europe-west (Zurich)

You may use a service account for the Cloud Scheduler. The best practice is to always use service accounts that have the least privileges appropriate for the job in question. In this example we will use `$SERVICE_ACCOUNT_SCHEDULER`. This email account follows this format: `service-account-name@project-id.iam.gserviceaccount.com`
If the account lacks the correct privileges, you'll see this message: *"Requested entity was not found. This command is authenticated as you@email.com which is the active account specified by the [core/account] property."*

```bash
SERVICE_ACCOUNT_SCHEDULER="PROJECT_NUMBER-compute@developer.gserviceaccount.com"

gcloud scheduler jobs create http yle-scraper-daily \
  --location europe-west1 \
  --schedule "0 3 * * *" \
  --uri "https://$REGION-run.googleapis.com/apis/run.googleapis.com/v1/namespaces/$PROJECT_ID/jobs/yle-scraper:run" \
  --http-method POST \
  --oauth-service-account-email $SERVICE_ACCOUNT_SCHEDULER
  
```
To test the scheduler:
```bash
gcloud scheduler jobs run yle-scraper-daily --location europe-west1

```

To list your scheduled jobs:
```bash
gcloud scheduler jobs list --location europe-west1

```

## Example output
See the example [here](https://storage.googleapis.com/yle-scraper-public-demo/xtxmarkets-fi.xml) or an excerpt below:
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