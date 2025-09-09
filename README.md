# locomotive

A Railway sidecar service for sending webhook events when new logs are received.

Nearly equivelant to Heroku's log drain, but for Railway.

With tailored support for:

- Datadog
- Axiom
- BetterStack
- Loki
- Sentry
- Papertrail

And more with the standard JSON and JSON Lines modes.

[![Deploy on Railway](https://railway.com/button.svg)](https://railway.com/new/template/locomotive)

## Configuration

Configuration is done through environment variables. See explanation and examples below.

**Notes:**

- Metadata such as the project, service, and environment names, along with their IDs, are automatically added to the logs that are sent under a `_metadata` attribute.

- Metadata is gathered on startup and then approximately every 10 to 20 minutes. If a project, service, or environment name has changed, the name in the metadata will not be correct until the locomotive refreshes its metadata.

- The root attributes in the HTTP logs are subject to change as Railway adds or removes attributes.

### All variables:

- `LOCOMOTIVE_RAILWAY_API_KEY` - Your Railway API token.

    **Required**.

    - Project level tokens do not work.
    - Team scoped tokens do not work.

    Generate a [Railway API Token](https://railway.com/account/tokens)

    </br>

- `LOCOMOTIVE_ENVIRONMENT_ID` - The ID of the environment your services are in.

    **Required**.

    - Auto-filled to the current environment ID.

    Make sure to deploy locomotive into the same environment as the services you want to monitor.

    [Railway Best Practices](https://docs.railway.com/overview/best-practices#deploying-related-services-into-the-same-project)

    Upon startup, Locomotive will verify that all the services exist within the set environment. If the environment does not exist, Locomotive will exit with an API error message.

    If that check fails with an unauthorized error, you are likely using the wrong kind of API token.

    </br>

- `LOCOMOTIVE_SERVICE_IDS` - The IDs of the services you want to monitor.

    **Required**.

    - Supports a single service ID.
    - Supports multiple service IDs, separated with a comma.

    Upon startup, locomotive will verify that all the services exist within the set environment. If any services do not exist, locomotive will exit with an error and provide a list of the missing services.

    If that check fails with an unauthorized error, you are likely using the wrong kind of API token.

    </br>

- `LOCOMOTIVE_WEBHOOK_URL` - The URL to send the webhook to.

    **Required**.

    - Example for Datadog: `https://http-intake.logs.datadoghq.com/api/v2/logs`
    - Example for Axiom: `https://api.axiom.co/v1/datasets/<DATASET_NAME>/ingest`
    - Example for BetterStack: `https://in.logs.betterstack.com`

    See [Provider specific setup](#provider-specific-setup) for more information.

    </br>

- `LOCOMOTIVE_ADDITIONAL_HEADERS` - Any additional headers to be sent with the request.

    **Optional**.

    - Useful for authentication. The string is in the format of a cookie, meaning each key-value pair is separated by a semicolon, and each key and value are separated by an equals sign.

    - Example for Datadog: `ADDITIONAL_HEADERS=DD-API-KEY=<DD_API_KEY>;DD-APPLICATION-KEY=<DD_APP_KEY>`
    - Example for Axiom/BetterStack: `ADDITIONAL_HEADERS=Authorization=Bearer <API_TOKEN>`

    See [Provider specific setup](#provider-specific-setup) for more information.

    </br>

- `LOCOMOTIVE_WEBHOOK_MODE` - The mode to use for the webhook.

    **Optional**.

    - Default: `json`
    
    Currently supported modes:

    - `json`
    - `jsonl`
    - `papertrail`
    - `datadog`
    - `axiom`
    - `betterstack`
    - `loki`
    - `sentry`

    </br>

- `LOCOMOTIVE_REPORT_STATUS_EVERY` - Reports the status of the locomotive every 5 seconds.

    **Optional**.

    - Default: `1m`
    - Format must be in the Golang `time.DurationParse` format
        - E.g. `10h`, `5h`, `10m`, `5m 5s`

    </br>

- `LOCOMOTIVE_ENABLE_HTTP_LOGS` - Enable transport of HTTP logs.

    **Optional**.

    - Default: `false`

    </br>

- `LOCOMOTIVE_ENABLE_DEPLOY_LOGS` - Enable transport of deploy logs.

    **Optional**.

    - Default: `true`

    </br>

### Provider specific setup:

#### Papertrail

- `LOCOMOTIVE_WEBHOOK_MODE` - `papertrail`
- `LOCOMOTIVE_WEBHOOK_URL` - `https://<PAPERTRAIL_HOSTNAME>/v1/logs/bulk`

    The hostname can be found by adding a new destination and then opening the usage instructions.
- `LOCOMOTIVE_ADDITIONAL_HEADERS` - `Authorization=Bearer <PAPERTRAIL_TOKEN>`

    The token can be found by adding a new destination and then opening the usage instructions.

    </br>

#### Datadog

- `LOCOMOTIVE_WEBHOOK_MODE` - `datadog`

- `LOCOMOTIVE_WEBHOOK_URL` - `https://http-intake.logs.datadoghq.com/api/v2/logs`

- `LOCOMOTIVE_ADDITIONAL_HEADERS` - `DD-API-KEY=<DD_API_KEY>;DD-APPLICATION-KEY=<DD_APP_KEY>`

    </br>

#### Axiom

- `LOCOMOTIVE_WEBHOOK_MODE` - `axiom`

- `LOCOMOTIVE_WEBHOOK_URL` - `https://api.axiom.co/v1/datasets/<DATASET_NAME>/ingest`

    The dataset name can be found by under the 'Datasets' tab in the Axiom UI.

- `LOCOMOTIVE_ADDITIONAL_HEADERS` - `Authorization=Bearer <API_TOKEN>`

    The API token can be generated from within your account settings under the 'API Tokens' tab.

    </br>

#### BetterStack

- `LOCOMOTIVE_WEBHOOK_MODE` - `betterstack`

- `LOCOMOTIVE_WEBHOOK_URL` - `https://<BETTERSTACK_HOSTNAME>`

    The hostname is generated when connecting a new source; choose HTTP.

    You can also find the hostname in source configuration.

- `LOCOMOTIVE_ADDITIONAL_HEADERS` - `Authorization=Bearer <TOKEN>`

    The token is generated when connecting a new source; choose HTTP.

    You can also find the token in source configuration.

    </br>

#### Loki

- `LOCOMOTIVE_WEBHOOK_MODE` - `loki`

- `LOCOMOTIVE_WEBHOOK_URL` - `https://<LOKI_HOSTNAME>/loki/api/v1/push`

    The hostname would depend on where you are running Loki.

    Or, with username and password authentication:

    `https://<USERNAME>:<PASSWORD>@<LOKI_HOSTNAME>/loki/api/v1/push`

    </br>

#### Sentry

- `LOCOMOTIVE_WEBHOOK_MODE` - `sentry`

- `LOCOMOTIVE_WEBHOOK_URL` - `https://<SENTRY_HOSTNAME>/api/<SENTRY_PROJECT_ID>/envelope/`

    The hostname can be found in the 'Client Keys (DSN)' section of the Sentry project settings, it will be the hostname of the given DSN.

    The project ID can be also be found in the 'Client Keys (DSN)' section of the Sentry project settings, it will be the path in the URL of the given DSN.

- `LOCOMOTIVE_ADDITIONAL_HEADERS` - `X-Sentry-Auth=Sentry sentry_key=<SENTRY_KEY>`

    The key can again be found in the 'Client Keys (DSN)' section of the Sentry project settings, it will be the user part of the given DSN.

    </br>