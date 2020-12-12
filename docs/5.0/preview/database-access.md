---
title: Teleport Database Access
description: Secure and Audited Access to Postgres Databases. Documentation to outline our preview.
---

# Teleport Database Access Preview

Teleport Database Access allows organizations to use Teleport as a proxy to
provide secure access to their databases while improving both visibility and
access control.

To find out whether you can benefit from using Database Access, see if you're
facing any of the following challenges in your organization:

* Do you need to protect and segment access to your databases?
* Do you need provide SSO and auditing for the database access?
* Do you have compliance requirements for data protection and access monitoring
  like PCI/FedRAMP?

If so, Database Access might help you solve some of these challenges.

## Features

With Database Access users can:

* Provide secure access to databases without exposing them over the public
  network through Teleport's reverse tunnel subsystem.
* Control access to specific database instances as well as individual
  databases and database users through Teleport's RBAC model.
* Track individual users' access to databases as well as query activity
  through Teleport's audit log.

## Diagram

The following diagram shows an example Database Access setup:

* Root cluster provides access to an onprem instance of PostgreSQL.
* Leaf cluster, connected to the root cluster, provides access to an
  onprem instance of MySQL and PostgreSQL-compatible AWS Aurora.
* Node connects another on-premise PostgreSQL instance (perhaps, a
  metrics database) via tunnel to the root cluster.

![Teleport database access diagram](../img/dbaccess.svg)

## Schedule

Teleport Database Access is under active development. The alpha release will
include support for PostgreSQL, including Amazon RDS and Aurora.

See [release schedule](./upcoming-releases.md#release-schedule).

## Configure PostgreSQL

### On-Prem PostgreSQL

!!! note
    This section explains how to configure an on-premise instance of PostgreSQL
    to work with Teleport Database access. For information about configuring
    AWS RDS/Aurora see the [section below](#aws-rdsaurora-postgresql).

#### Create Certificate/Key Pair

Teleport uses mutual TLS authentication so PostgreSQL server must be configured
with Teleport's certificate authority and certificate/key pair that Teleport
can validate.

To create these secrets, use `tctl auth sign` command. Note that it requires a
running Teleport cluster and [should be run](https://goteleport.com/teleport/docs/architecture/overview/#tctl)
on the auth server.

```sh
$ tctl auth sign --format=db --host=db.example.com --out=server --ttl=8760h
```

Flag descriptions:

* `--format=db`: instructs the command to produce secrets in the format suitable
  for configuring a database server.
* `--host=db.example.com`: server name to encode in the certificate, should
  match the hostname Teleport will be connecting to the database at.
* `--out=server`: name prefix for output files.
* `--ttl=8760h`: certificate validity period.

The command will create 3 files: `server.cas` with Teleport's certificate
authority and `server.crt`/`server.key` with generated certificate/key pair.

#### Configure PostgreSQL Server

To configure PostgreSQL server to accept TLS connections, add the following
to PostgreSQL configuration file `postgresql.conf`:

```conf
ssl = on
ssl_cert_file = '/path/to/server.crt'
ssl_key_file = '/path/to/server.key'
ssl_ca_file = '/path/toa/server.cas'
```

See [Secure TCP/IP Connections with SSL](https://www.postgresql.org/docs/current/ssl-tcp.html)
in PostgreSQL documentation for more details.

Additionally, PostgreSQL should be configured to require client certificate
authentication from clients connecting over TLS. This can be done by adding
the following entries to PostgreSQL host-based authentication file `pg_hba.conf`:

```conf
hostssl all             all             ::/0                    cert
hostssl all             all             0.0.0.0/0               cert
```

See [The pg_hba.conf File](https://www.postgresql.org/docs/current/auth-pg-hba-conf.html)
in PostgreSQL documentation for more details.

### AWS RDS/Aurora PostgreSQL

!!! note
    This section explains how to configure a PostgreSQL-flavored instance of
    AWS RDS or Aurora database to work with Teleport database access. For
    information about configuring an on-prem PostgreSQL see the [section above](#on-prem-postgresql).

Teleport Database Access for AWS RDS and Aurora uses IAM authentication which
can be enabled with the following steps.

#### Enable IAM Authentication

Open [Amazon RDS console](https://console.aws.amazon.com/rds/) and create a new
database instance with IAM authentication enabled, or modify an existing one to
turn it on. Make sure to use PostgreSQL database type.

See [Enabling and disabling IAM database authentication](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/UsingWithRDS.IAMDBAuth.Enabling.html)
for more information.

#### Create IAM Policy

To allow Teleport database service to log into the database instance using auth
token, create an IAM policy and attach it to the user whose credentials the
database service will be using, for example:

```json
{
   "Version": "2012-10-17",
   "Statement": [
      {
         "Effect": "Allow",
         "Action": [
             "rds-db:connect"
         ],
         "Resource": [
             "arn:aws:rds-db:us-east-2:1234567890:dbuser:cluster-ABCDEFGHIJKL01234/*"
         ]
      }
   ]
}
```

See [Creating and using an IAM policy for IAM database access](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/UsingWithRDS.IAMDBAuth.IAMPolicy.html)
for more information.

#### Create Database User

Database users must have a `rds_iam` role in order to be allowed access. For
PostgreSQL:

```sql
CREATE USER alice;
GRANT rds_iam TO alice;
```

See [Creating a database account using IAM authentication](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/UsingWithRDS.IAMDBAuth.DBAccounts.html)
for more information.

## Configure Teleport

First, head over to the Teleport [downloads page](https://gravitational.com/teleport/download/)
and download the latest version of Teleport.

!!! warning
    As of this writing, no Teleport release with Database Access has been
    published yet.

Follow the installation [instructions](https://gravitational.com/teleport/docs/installation/).

### Start Auth/Proxy Service

Create a configuration file for a Teleport service that will be running
auth and proxy servers:

```yaml
teleport:
  data_dir: /var/lib/teleport
  nodename: test
auth_service:
  enabled: "yes"
  cluster_name: "test"
  listen_addr: 0.0.0.0:3025
  tokens:
  - proxy,node,database:cbdeeab9-6f88-436d-a673-44d14bd86bb7
proxy_service:
  enabled: "yes"
  listen_addr: 0.0.0.0:3023
  web_listen_addr: 0.0.0.0:3080
  tunnel_listen_addr: 0.0.0.0:3024
  public_addr: teleport.example.com:3080
ssh_service:
  enabled: "no"
```

Start the service:

```sh
$ teleport start -d --config=/path/to/teleport.yaml
```

### Start Database Service with CLI Flags

For a quick try-out, Teleport database service doesn't require a configuration
file and can be launched using a single CLI command:

```sh
$ teleport start -d \
   --roles=database \
   --token=cbdeeab9-6f88-436d-a673-44d14bd86bb7 \
   --auth-server=teleport.example.com:3080 \
   --db-name=test \
   --db-protocol=postgres \
   --db-uri=db.example.com:5432 \
   --labels=env=test
```

Note that the `--auth-server` flag must point to cluster's proxy endpoint
because database service always connects back to the cluster over reverse
tunnel framework.

Instead of using a static auth token, a short-lived dynamic token can also
be generated for a database service:

```sh
$ tctl tokens add \
    --type=database \
    --db-name=test \
    --db-protocol=postgres \
    --db-uri=db.example.com:5432
```

### Start Database Service with Config File

Below is the example of a database service configuration file that proxies
a single AWS Aurora database:

```yaml
teleport:
  data_dir: /var/lib/teleport-db
  nodename: test
  # Auth token to connect to the auth server.
  auth_token: cbdeeab9-6f88-436d-a673-44d14bd86bb7
  # Proxy address to connect to. Note that it has to be the proxy address
  # because database service always connects to the cluster over reverse
  # tunnel framework.
  auth_servers:
  - teleport.example.com:3080
db_service:
  enabled: "yes"
  # This section contains definitions of all databases proxied by this
  # service, can contain multiple.
  databases:
    # Name of the database proxy instance, used to reference in CLI.
  - name: "aurora"
    # Free-form description of the database proxy instance.
    description: "AWS Aurora instance of PostgreSQL 13.0"
    # Database protocol.
    protocol: "postgres"
    # Database address, example of a AWS Aurora endpoint in this case.
    uri: "postgres-aurora-instance-1.xxx.us-east-1.rds.amazonaws.com:5432"
    # AWS specific configuration, only required for RDS and Aurora.
    aws:
      # Region the database is deployed in.
      region: us-east-1
    # Labels to assign to the database, used in RBAC.
    labels:
      env: dev
auth_service:
  enabled: "no"
ssh_service:
  enabled: "no"
proxy_service:
  enabled: "no"
```

!!! tip
    Single Teleport process can run multiple different services, for example
    multiple database access proxies as well as run other services such as
    SSH service and application access proxy.

Start the database service:

```sh
$ teleport start -d --config=/path/to/teleport-db.yaml
```

### AWS Credentials

When setting up Teleport database service with AWS RDS or Aurora, it must have
an IAM role allowing it to connect to that particular database instance. An
example of such policy is shown in the [AWS RDS/Aurora](#aws-rdsaurora-postgresql)
section above. See [Creating and using an IAM policy for IAM database access](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/UsingWithRDS.IAMDBAuth.IAMPolicy.html)
in AWS documentation.

Teleport database service uses default credential provider chain to find AWS
credentials. See [Specifying Credentials](https://docs.aws.amazon.com/sdk-for-go/v1/developer-guide/configuring-sdk.html#specifying-credentials)
for more information.

## Connect

Once the database service has joined the cluster, login to see the available
databases:

```sh
$ tsh login --proxy=teleport.example.com:3080
$ tsh db ls
Name   Description Labels
------ ----------- --------
aurora AWS Aurora  env=dev
```

Note that you will only be able to see databases your role has access to. See
[RBAC](#rbac) section for more details.

To connect to a particular database server, first retrieve credentials from
Teleport using `tsh db login` command:

```sh
$ tsh db login aurora
```

!!! tip
    You can be logged into multiple databases simultaneously.

You can optionally also specify default database name and user to use when
connecting to the database instance:

```sh
$ tsh db login --db-user=postgres --db-name=postgres aurora
```

When logging into a PostgreSQL database, `tsh` automatically configures a section
in the [connection service file](https://www.postgresql.org/docs/current/libpq-pgservice.html)
with the name of `<cluster-name>-<database-service-name>`.

Suppose the cluster name is "root", then you can connect to the database using
the following `psql` command:

```sh
# Use default database user/name.
$ psql "service=root-aurora"
# Specify database name/user explicitly.
$ psql "service=root-aurora user=alice dbname=metrics"
```

To log out of the database and remove credentials:

```sh
# Log out of a particular database instance.
$ tsh db logout aurora
# Log out of all database instances.
$ tsh db logout
```

## RBAC

Teleport's "role" resource provides the following instruments for restricting
the database access:

```yaml
kind: role
version: v3
metadata:
  name: developer
spec:
  allow:
    # Label selectors for database instances this role has access to. These
    # will be matched against the static/dynamic labels set on the database
    # service.
    db_labels:
      environment: ["dev", "stage"]
    # Database names this role has connect to. Note: this is not the same as
    # the "name" field in "db_service", this is the database names within a
    # particular database instance.
    db_names: ["main", "metrics", "postgres"]
    # Database users this role can connect as.
    db_users: ["alice", "bob"]
```

Similar to other role fields, these support templating variables to allow
propagating information from identity providers:

```yaml
spec:
  allow:
    db_names: ["{% raw %}{{internal.db_names}}{% endraw %}", "{% raw %}{{external.xxx}}{% endraw %}"]
    db_users: ["{% raw %}{{internal.db_users}}{% endraw %}", "{% raw %}{{external.yyy}}{% endraw %}"]
```

See general [RBAC](/enterprise/ssh-rbac) documentation for more details.

## Demo

<video autoplay loop muted playsinline controls style="width:100%">
  <source src="https://goteleport.com/teleport/videos/database-access-preview/dbaccessdemo.mp4" type="video/mp4">
  <source src="https://goteleport.com/teleport/videos/database-access-preview/dbaccessdemo.webm" type="video/webm">
Your browser does not support the video tag.
</video>

## RFD

Please refer to the [RFD document](https://github.com/gravitational/teleport/blob/master/rfd/0011-database-access.md)
for a more in-depth description of the feature scope and design.

## Feedback

We value your feedback. Please schedule a Zoom call with us to get in-depth
demo and give us feedback using [this](https://calendly.com/benarent/teleport-database-access?month=2020-11)
link.

If you found a bug, please create a [Github
Issue](https://github.com/gravitational/teleport/issues/new/choose).
