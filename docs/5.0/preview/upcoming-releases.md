---
title: Teleport Upcoming Releases
description: A timeline of upcoming Teleport releases.
---

# Upcoming Teleport Releases

The teleport team delivers a new major release roughly every 3 months.

## Teleport 6.0 "San Diego"

The team open sources role based access controls, implements Database Access, Session Termination
and Access Workflows UI.


### Release Schedule

| Version              | Date              | Description
|---------------------|-------------------|---------------------------
| First alpha          | Jan 15th, 2021    | Good for testing and demos.
| First beta           | Feb 1st, 2021     | Deploy on staging.
| Release              | March 1st, 2021   | Good to go for production.

### Features

You can find the full list of fixes and features in the
[Github milestone](https://github.com/gravitational/teleport/milestone/33).

|Feature                              | Editions          | Description
|-------------------------------------|-------------------|-----------------------------------
| Database Access                     | All               | SSO into PostgreSQL or MySQL. Use AWS Aurora RDS or on-premises. Read [more here](./database-access.md).
| OSS RBAC                            | OSS               | Open source role based access controls. Check out the [design doc](https://github.com/gravitational/teleport/blob/master/rfd/0007-rbac-oss.md) and [issue 4136](https://github.com/gravitational/teleport/issues/4136).
| Terraform Provider                  | All               | Configure Teleport without UI using Terraform provider. More details [here](https://github.com/gravitational/teleport-plugins/projects/3#card-49866475).
| Dual Authorization Workflows        | Enterprise, Cloud | Request multiple users to review and approve access requests. Find out more in [issue 5007](https://github.com/gravitational/teleport/issues/5007).
| U2F for Kubernetes and SSH sessions | All               | Adds an option to authorize with 2nd factor when connecting to a node/k8s cluster. Details in [issue 3828](https://github.com/gravitational/teleport/issues/3878).
| Access Workflows UI                 | Enterprise, Cloud | Review access requests and assume roles in the UI. Some mockups are in [issue 4937](https://github.com/gravitational/teleport/issues/4937).
| Client libraries and API            | All               | Use Go to create access workflows. Review the [design doc](https://github.com/gravitational/teleport/pull/4746) and [issue 4763](https://github.com/gravitational/teleport/issues/4763).

## Semantic Versioning

Teleport follows [semantic versioning](https://semver.org/) for pre-releases and releases.

**Pre-releases**

Pre-releases have suffixes `-alpha`, `-beta` and `-rc`.
They are not ready for production:

* You can use alpha releases such as `5.0.1-alpha.1` for trying new features.
  Things can break and new changes may not be backwards-compatible.

* Teleport `beta` releases, such as `5.0.1-beta.2` are suitable for staging environments.
  We are unlikely to change the APIs while we are ironing out bugs and UX glitches.

* We mark release candidates as `5.0.1-rc.1` coding and bug fixes are finished.
  The team is going through the manual test plan to find any regressions.

**Releases**

Releases are ready for production use.

* Releases `5.0.0` and `6.0.0` are major releases. We publish 4 major releases each year.
Read more about upgrades and compatibility [here](../admin-guide.md#component-compatibility).

* Releases `5.1.0` are minor releases. They contain minor backwards-compatible improvements and backports.

* Versions like `5.0.1` are quick patches. They contain backwards-compatible fixes.
