# Gravitational Web Applications and Packages

This directory contains the source code for:

- the web UIs served by the `teleport` server
  - [`packages/teleport`](packages/teleport)
- the Electron app of [Teleport Connect](https://goteleport.com/connect/)
  - [`packages/teleterm`](packages/teleterm)

The code is organized in terms of independent packages (workspaces) which reside in
the [packages directory](packages).

## Getting Started with Teleport Web UI

You can make production builds locally, or you can use Docker to do that.

### Local Build

Install Node.js (you can take the version by executing 
`make -C build.assets print-node-version` from the root directory).
After that, run `corepack enable pnpm` to enable installing a package manager.

Then you need to download and initialize JavaScript dependencies.

```
pnpm install
```

You will also need the following tools installed:
* The `Rust` and `Cargo` version in [build.assets/Makefile](https://github.com/gravitational/teleport/blob/master/build.assets/versions.mk#L11) (search for `RUST_VERSION`) are required.
* The [`wasm-pack`](https://github.com/rustwasm/wasm-pack) version in [build.assets/Makefile](https://github.com/gravitational/teleport/blob/master/build.assets/versions.mk#L12) (search for `WASM_PACK_VERSION`) is required:
  `curl https://rustwasm.github.io/wasm-pack/installer/init.sh -sSf | sh`
* [`binaryen`](https://github.com/WebAssembly/binaryen) (which contains `wasm-opt`) is required to be installed manually
    on linux aarch64 (64-bit ARM). You can check if it's already installed on your system by running `which wasm-opt`. If not you can install it like `apt-get install binaryen` (for Debian-based Linux). `wasm-pack` will install this automatically on other platforms.

To build the Teleport open source version

```
pnpm build-ui-oss
```

The resulting output will be in the `webassets` folder.

### Docker Build

To build the Teleport community version

```
make docker-ui
```

## Getting Started with Teleport Connect

See [`README.md` in `packages/teleterm`](packages/teleterm#readme).

## Development

### Local HTTPS

To run `vite` for either Teleport or Teleport enterprise, you'll need to generate local
self-signed certificates. The recommended way of doing this is via [mkcert](https://github.com/FiloSottile/mkcert).

You can install mkcert via

```
brew install mkcert
```

After you've done this, run:

```
mkcert -install
```

This will generate a root CA on your machine and automatically trust it (you'll be prompted for your password).

Once you've generated a root CA, you'll need to generate a certificate for local usage.

Run the following from the `web/` directory, replacing `localhost` if you're using a different hostname.

```
mkdir -p certs && mkcert -cert-file certs/server.crt -key-file certs/server.key localhost "*.localhost"
```

(Note: the `certs/` directory in this repo is ignored by git, so you can place your certificate/keys
in there without having to worry that they'll end up in a commit.)

#### Certificates in an alternative location

If you already have local certificates, you can set the environment variables:

- `VITE_HTTPS_CERT` **(required)** - absolute path to the certificate
- `VITE_HTTPS_KEY` **(required)** - absolute path to the key

You can set these in your `~/.zshrc`, `~/.bashrc`, etc.

```
export VITE_HTTPS_CERT=/Users/you/certs/server.crt
export VITE_HTTPS_KEY=/Users/you/certs/server.key
```

### Web UI

To avoid having to install a dedicated Teleport cluster,
you can use a local development server which can proxy network requests
to an existing cluster.

For example, if `https://example.com:3080` is the URL of your cluster then:

To start your local Teleport development server

```
PROXY_TARGET=example.com:3080 pnpm start-teleport
```

If you're running a local cluster at `https://localhost:3080`, you can just run

```
pnpm start-teleport
```

This service will serve your local javascript files and proxy network
requests to the given target.

> Keep in mind that you have to use a local user because social
> logins (google/github) are not supported by development server.

### Unit-Tests

We use [jest](https://jestjs.io/) as our testing framework.

To run all jest unit-tests:

```
pnpm test
```

To run jest in watch-mode

```
pnpm tdd
```

### Interactive Testing

We use [Storybook](https://storybook.js.org/) for our interactive testing.
It allows us to browse our component library, view the different states of
each component, and interactively develop and test components.

> [!IMPORTANT]
> In order to start Storybook, you need to have certs in `web/certs`.
> See [Local HTTPS](#local-https) for how to set them up.

To start Storybook:

```
pnpm storybook
```

This command will open a new browser window with Storybook in it. There
you will see components from all packages so it makes it faster to work
and iterate on shared functionality.

### Browser compatibility

We are targeting last 2 versions of all major browsers. To quickly find out which ones exactly, use the following command:

```
pnpm dlx browserslist 'last 2 chrome version, last 2 edge version, last 2 firefox version, last 2 safari version'
```

### Setup Prettier on VSCode

1. Install plugin: https://github.com/prettier/prettier-vscode
1. Go to Command Palette: CMD/CTRL + SHIFT + P (or F1)
1. Type `open settings`
1. Select `Open Settings (JSON)`
1. Include the below snippet and save:

```js

    // Set the default
    "editor.formatOnSave": false,
    // absolute config path
    "prettier.configPath": ".prettierrc.js",
    // enable per-language
    "[html]": {
        "editor.formatOnSave": true,
        "editor.defaultFormatter": "esbenp.prettier-vscode"
    },
    "[javascript]": {
        "editor.formatOnSave": true,
        "editor.defaultFormatter": "esbenp.prettier-vscode"
    },
    "[javascriptreact]": {
        "editor.formatOnSave": true,
        "editor.defaultFormatter": "esbenp.prettier-vscode",
    },
    "[typescript]": {
        "editor.formatOnSave": true,
        "editor.defaultFormatter": "esbenp.prettier-vscode"
    },
    "[typescriptreact]": {
        "editor.formatOnSave": true,
        "editor.defaultFormatter": "esbenp.prettier-vscode",
    },
    "[json]": {
        "editor.formatOnSave": true,
        "editor.defaultFormatter": "vscode.json-language-features"
    },
    "[jsonc]": {
        "editor.formatOnSave": true,
        "editor.defaultFormatter": "vscode.json-language-features"
    },
    "[markdown]": {
        "editor.formatOnSave": true,
        "editor.defaultFormatter": "esbenp.prettier-vscode",
    },
    "editor.tabSize": 2,
```

### MFA Development

#### Local cluster with nip.io

When developing MFA sections of the codebase, you may need to configure the `teleport.yaml` of your target teleport cluster to accept hardware keys registered over the local development setup. Webauthn can get tempermental if you try to use localhost as your `rp_id`, but you can get around this by using https://nip.io/. For example, if you want to configure optional `webauthn` mfa, you can set up your auth service like so:

```yaml
auth_service:
  authentication:
    type: local
    second_factor: optional
    webauthn:
      rp_id: proxy.127.0.0.1.nip.io

proxy_service:
  enabled: yes
  # setting public_addr is optional, useful if using different port e.g. 8080 instead of default 3080
  public_addr: ['proxy.127.0.0.1.nip.io']
```

Then start the dev server like `PROXY_TARGET=https://proxy.127.0.0.1.nip.io:3080 pnpm start-teleport` and access it at https://proxy.127.0.0.1.nip.io:8080.

#### Local cluster with /etc/hosts

Unlike the method above, this one requires no changes to an existing local cluster.

If you have entries for your cluster in `/etc/hosts` and your cluster is configured to use something
like `teleport.test:3080` as the public address of the proxy service, you can just set a proxy
target to that public address. Then instead of accessing the Vite proxy at `localhost:8080`, you can
access it at `teleport.test:8080`. MFA will work fine since RP ID will still be `teleport.test`.

#### Remote cluster with socat

Update `/etc/hosts` to override DNS resolution of your proxy addr, e.g my proxy is at alpha.devteleport.com:

```
::1 alpha.devteleport.com
127.0.0.1 alpha.devteleport.com
```

Assuming you've properly configured DNS with a wildcard subdomain record: start a transparent TCP
proxy that forwards to the Teleport proxy using whatever sub-sub domain (has to be nested twice to
avoid being routed to teleport app access). AWS route53 wildcard records work for any subdomain
nesting level. Alternatively, hardcode the real teleport proxy's IP. e.g. with `socat` (`brew install
socat`):

```
sudo socat -d2 TCP-LISTEN:443,reuseaddr,fork TCP:x.y.alpha.devteleport.com:443
```

Generate certs for your proxy's domain as described in [Local HTTPS](#local-https). Start the local
dev UI server:

```
PROXY_TARGET=alpha.devteleport.com:443 pnpm start-teleport
```

This makes your browser, tsh, and anything else on your system that respects `/etc/hosts` think it's
talking to your proxy directly, but it's actually being forwarded without TLS termination by
`socat`, allowing Webauthn to work as well as TLS verification. You can now go to either port 443 or
3000 with your actual teleport proxy address - 443 will go to the remote proxy's web UI, 3000 will
go to the local dev web UI. This can be used to run a dev web UI even for Teleport clusters you do
not own.

### Adding Packages/Dependencies

We use [pnpm workspaces](https://pnpm.io/workspaces) to manage dependencies.

To add a package to the workspace, run `pnpm --filter=<workspace-name> add <package-name>`.
Alternatively, you can add a line to the workspace's `package.json` file and then 
run `pnpm install` (or `pnpm i`) from the root of this repository.

Dependencies should generally be added to the specific workspaces that use them.
For instance, if you need a new dependency for Web UI, add it to `packages/teleport/package.json`.
Similarly, if you're adding a new ESLint plugin, include it in `packages/build/package.json`.
If a dependency is imported in, for example, `packages/shared` and `packages/teleterm`
add it to both of them (that's what we'd have to do if they were real packages
published to a registry).

However, there are cases where dependencies should be added not in a workspace, but
in the root `package.json`:

1. Dependencies imported in `e` code (we can't declare dependencies in `e/web/teleport/package.json`
to avoid generating a different lockfile when `e` isn't cloned).
For example, `react` - it is imported in every package (in `e` too), so it
needs to be kept in the root.
2. CLI tools which are run from the root of the repo, like `prettier`.

### Adding an Audit Event

When a new event is added to Teleport, the web UI has to be updated to display it correctly:

1. Add a new entry to [`eventCodes`](https://github.com/gravitational/webapps/blob/8a0201667f045be7a46606189a6deccdaee2fe1f/packages/teleport/src/services/audit/types.ts).
2. Add a new entry to [`RawEvents`](https://github.com/gravitational/webapps/blob/8a0201667f045be7a46606189a6deccdaee2fe1f/packages/teleport/src/services/audit/types.ts) using the event you just created as the key. The fields should match the fields of the metadata fields on `events.proto` on Teleport repository.
3. Add a new entry in [Formatters](https://github.com/gravitational/webapps/blob/8a0201667f045be7a46606189a6deccdaee2fe1f/packages/teleport/src/services/audit/makeEvent.ts) to format the event on the events table. The `format` function will receive the event you added to `RawEvents` as parameter.
4. Define an icon to the event on [`EventIconMap`](https://github.com/gravitational/webapps/blob/8a0201667f045be7a46606189a6deccdaee2fe1f/packages/teleport/src/Audit/EventList/EventTypeCell.tsx).
5. Add an entry to the [`events`](https://github.com/gravitational/webapps/blob/8a0201667f045be7a46606189a6deccdaee2fe1f/packages/teleport/src/Audit/fixtures/index.ts) array so it will show up on the [`AllEvents` story](https://github.com/gravitational/webapps/blob/8a0201667f045be7a46606189a6deccdaee2fe1f/packages/teleport/src/Audit/Audit.story.tsx)
6. Check fixture is rendered in storybook, then update snapshot for `Audit.story.test.tsx` using `pnpm test-update-snapshot`.

You can see an example in [this pr](https://github.com/gravitational/webapps/pull/561).
