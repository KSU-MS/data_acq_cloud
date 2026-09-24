# Deploy the full app through host Tailscale Serve

The repository-root `compose.yml` runs the Go backend, MongoDB, MinIO and bucket
initialization. The frontend is deployed separately through GitHub Pages.
Tailscale runs on the **deployment host**, not in a container.
Run the commands below from the `data_acq_cloud` repository root.

```text
Authorized tailnet browser
  HTTPS :443  -> host Tailscale Serve -> 127.0.0.1:8080 -> Go API
  HTTPS :8443 -> host TLS termination -> 127.0.0.1:9000 -> MinIO S3

Go API -> mongodb:27017 (MongoDB protocol)
       -> http://minio:9000 (S3 object operations)
```

MongoDB has no host ports. The Go API, S3 API and MinIO console
bind only to host loopback. The console at `127.0.0.1:9001` is for administration;
it is not included in the Serve configuration. A single project-scoped Docker
bridge connects the services. No external `query-shared` network is required.
`expose` is not needed for containers on that network to communicate.

## 1. Prepare the deployment host and configuration

Use Docker Compose and a current Tailscale installation supporting
`--tls-terminated-tcp`. Join the host to your
tailnet, enable MagicDNS and HTTPS certificates in the Tailscale admin console,
and allow the intended users/groups to access the host on TCP ports 443 and 8443.
Serve honors tailnet access rules; existing broad rules may already allow more
users than you intend. Do not enable Funnel for these endpoints.

Create the root environment file, without replacing an existing one:

```bash
cp -n .env.example .env
chmod 600 .env
openssl rand -hex 24
openssl rand -hex 24
```

Edit `.env` and use the two different generated passwords for MongoDB and MinIO.
The MongoDB username/password are interpolated into a URI; the sample username
and a hex password avoid characters needing percent-encoding. If you use other
characters, provide a correctly URI-encoded connection string in Compose while
keeping MongoDB's initialization password unencoded.

On the **actual deployment host**, obtain its MagicDNS name:

```bash
sudo tailscale status --json | jq -r '.Self.DNSName | rtrimstr(".")'
```

If the output is `lib-o-yap.tail937b5a.ts.net`, set:

```dotenv
AWS_S3_PUBLIC_ENDPOINT=https://lib-o-yap.tail937b5a.ts.net:8443
```

This is a placeholder example, not a hostname created by the repository. Copy
your actual output, without its trailing dot. Keep HTTPS and port 8443; append
neither a bucket name nor `/s3`. The internal endpoint is fixed in Compose as
`AWS_S3_ENDPOINT=http://minio:9000`.

The frontend is built and deployed by GitHub Pages. Set its Actions secret
`VITE_API_URL` to `https://lib-o-yap.tail937b5a.ts.net` and allow
`https://ksu-ms.github.io` in the backend CORS configuration. MPS remains
disabled; opening the frontend's MPS dialog will not discover runnable scripts.

## 2. Build and start the containers

```bash
docker compose config --quiet
docker compose up -d --build --wait
docker compose ps -a
curl --fail http://127.0.0.1:8080/ping
```

Startup waits for authenticated MongoDB readiness, successful bucket creation,
and the backend's HTTP health check. `minio-create-bucket` exiting with code 0
is expected. A failed initializer blocks backend startup; inspect its logs and
rerun `up` after fixing the cause. The bucket starts private.

If testing on the same machine as `docker/docker-compose_local.yml`, its MinIO
ports 9000/9001 conflict with this stack. Let processing finish, then stop the
local stack with its explicit `-f` argument before starting production. Its
`mcap-local-test` volumes are separate. Do not change the production Compose
project name casually: it determines the names of the persistent volumes.

## 3. Configure Tailscale Serve on the host

Inspect `sudo tailscale serve status` first if this host already serves other
apps. The following commands configure ports 443 and 8443 for this app:

```bash
sudo tailscale serve --bg --https=443 http://127.0.0.1:8080
sudo tailscale serve --bg --tls-terminated-tcp=8443 tcp://127.0.0.1:9000
sudo tailscale serve status
```

The background configuration persists across restarts. Ensure the host's
Tailscale and Docker services start on boot. Compose does not install Tailscale,
join the tailnet, change access rules or run these host commands automatically.

The S3 forwarder terminates TLS and then forwards the original HTTP bytes.
Consequently MinIO receives the signed `Host: <your-hostname>:8443`, object path
and query unchanged. There is no S3 hostname substitution or path-prefix rewrite.

## 4. Verify from a second authorized tailnet device

Open your GitHub Pages site and upload a representative MCAP. The browser calls
the backend through `https://<your-hostname>`. Wait for `Completed job` in:

```bash
docker compose logs -f cloud_webserver_v2
```

Query `https://<your-hostname>/api/v2/mcaps/` and download the run's MCAP, HDF5
and PNG artifacts using their complete `signed_url` values. They must start with
`https://<your-hostname>:8443/<bucket>/`, never `http://minio:9000`. Quotes around
signed URLs are necessary when using curl because the query contains `&`:

```bash
curl --fail --location 'PASTE_COMPLETE_SIGNED_URL' --output downloaded.mcap
```

Compare the downloaded MCAP's SHA-256 with the original. Inspect HDF5 samples and
plots as described in `README-local.md`; object existence and job completion alone
do not prove every signal decoded correctly. Links expire after ten minutes;
query the API again for fresh URLs. A 403 `SignatureDoesNotMatch` calls for checking
the exact hostname/port and any additional proxies; connection failure calls for
checking Serve status, MagicDNS, HTTPS and tailnet access rules.

## Updates, data and administration

To update the backend, pull the reviewed revision and run:

```bash
docker compose up -d --build --wait
```

If you change `AWS_S3_PUBLIC_ENDPOINT`, recreate the backend with
`docker compose up -d --force-recreate cloud_webserver_v2` and request fresh URLs.
The in-memory processing queue is not durable: allow uploads to finish before
restarting/rebuilding, and re-upload unfinished jobs afterward. The `PRODUCTION`
setting also retains the existing `/data/run_metadata` and `/mps_data` mounts;
MinIO remains the store used for browser downloads.

MongoDB, MinIO, upload working files and backend logs use persistent named
volumes. `docker compose down` preserves those volumes. `down --volumes` deletes
them and must not be used for ordinary upgrades. Back up MongoDB and MinIO data
off-host and verify restores; named volumes are not backups. Existing MongoDB
volumes retain their existing users/passwords: changing initialization variables
does not rotate credentials. Keep the previous production project name and
credentials when migrating an existing installation.

For MongoDB administration without publishing a database port:

```bash
docker compose exec mongodb sh -c 'exec mongosh --username "$MONGO_INITDB_ROOT_USERNAME" --authenticationDatabase admin'
```

Enter the configured password when prompted, then inspect `vehicle_data_db`.
For remote MinIO-console administration, use an SSH tunnel to host loopback.
The initial configuration uses MongoDB/MinIO root credentials for the backend;
a later hardening step is separate application users scoped to this database
and bucket.

References: [Tailscale Serve](https://tailscale.com/docs/reference/tailscale-cli/serve),
[Compose build configuration](https://docs.docker.com/reference/compose-file/build/),
[Compose startup ordering](https://docs.docker.com/compose/how-tos/startup-order/).
