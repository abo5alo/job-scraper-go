# Deploying to AWS

This runs the whole app on one small EC2 server: Postgres, the API, the
daily scraper, and [Caddy](https://caddyserver.com) in front for HTTPS. It's
the same Docker Compose setup as local development, plus
[docker-compose.prod.yml](../docker-compose.prod.yml), which keeps the
database and API off the internet and adds Caddy.

It takes about 20 minutes. A week of running costs a few dollars, which a new
AWS account's free credits cover.

## 1. Launch a server

In the AWS console, open **EC2** and click **Launch instance**:

| Setting | Value |
|---|---|
| Name | `job-scraper` |
| Image | **Ubuntu Server 24.04 LTS** |
| Instance type | **t3.micro** (free tier eligible; 1 GB of memory, step 3 adds swap) |
| Key pair | **Create new key pair**, type RSA, format `.pem`. Keep the downloaded file. |
| Network settings | Tick **Allow SSH traffic** (from **My IP**), **Allow HTTPS traffic** and **Allow HTTP traffic** from the internet |
| Storage | **20 GiB** gp3 |

Launch it, open the instance, and note its **Public IPv4 address**, for
example `3.120.45.6`.

Port 80 is needed even though the site uses HTTPS: Caddy proves it controls
the address over port 80 to get its certificate, then redirects HTTP visitors
to HTTPS.

## 2. Connect to it

The easiest way is in the browser: select the instance, click **Connect**,
then **Connect** again on the **EC2 Instance Connect** tab.

Or from your own terminal, with the key file you downloaded:

```sh
ssh -i job-scraper.pem ubuntu@3.120.45.6
```

Every command below runs on the server.

## 3. Install Docker and add swap

```sh
# 2 GB of swap, so compiling the Go programs fits in 1 GB of memory
sudo fallocate -l 2G /swapfile && sudo chmod 600 /swapfile
sudo mkswap /swapfile && sudo swapon /swapfile
echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab

# Docker's official install script, then let the ubuntu user run docker
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker ubuntu
```

Log out (`exit`) and connect again, so the `docker` group applies.

## 4. Get the code and configure it

```sh
git clone https://github.com/abo5alo/job-scraper-go.git
cd job-scraper-go
cp .env.example .env
```

Fill in `.env`. The password is generated for you; replace the address with
your server's public IP, written with dashes:

```sh
sed -i "s/^DB_PASSWORD=.*/DB_PASSWORD=$(openssl rand -hex 16)/" .env
sed -i "s/^SITE_ADDRESS=.*/SITE_ADDRESS=3-120-45-6.sslip.io/" .env
```

[sslip.io](https://sslip.io) is a free service that turns
`3-120-45-6.sslip.io` into the address `3.120.45.6`, so the site gets a name,
and with it an HTTPS certificate, without buying a domain. If you have a
domain, point it at the server and use that instead.

## 5. Start it

```sh
docker compose up -d --build     # build and start everything (a few minutes the first time)
docker compose run --rm scraper  # fill the database now instead of waiting for 03:00 UTC (~6 minutes)
```

Then open **https://3-120-45-6.sslip.io** (with your IP). The scheduler
scrapes again every day at 03:00 UTC.

## Day to day

```sh
docker compose ps                  # what's running
docker compose logs -f scheduler   # watch the daily scrape
docker compose logs caddy          # if HTTPS isn't working
git pull && docker compose up -d --build   # deploy new code
```

Don't stop the instance from the console: a stopped and restarted instance
gets a new public IP, and the site's address would point at nothing. Reboots
are fine, everything restarts by itself.

## When you're done

In EC2, select the instance and choose **Instance state → Terminate**. That
deletes the server and its disk, so the charges stop. Then check:

- **Elastic IPs**: there shouldn't be any. You only have one if you made one,
  and an unattached one is still billed.
- **Billing → Bills**, a day later, to confirm nothing is still running.
