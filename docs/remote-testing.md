# Remote testing through Authentik

`camspeak check-remote` performs only GET requests to `/api/health` and `/api/cameras`. It checks JSON response shape and connectivity without playing audio, changing configuration, or opening the local database. It does not prove authentication is enforced: public endpoints can also pass.

## Existing provider-issued bearer token

Use a token issued for the CamSpeak proxy provider/application, with access limited by its application policies. An Authentik administration API token or a token issued only for Klipbord is not interchangeable with this token. Do not put token values in command arguments, source control, screenshots, or chat.

Store the token in a private file outside the checkout, set its permissions to `600`, then run:

```sh
go run . check-remote --server https://camspeak.example.com --token-file /absolute/path/to/private-token
```

Alternatively set `CAMSPEAK_SERVER` and `CAMSPEAK_ACCESS_TOKEN` locally using your secret manager. An explicit token file takes precedence. The command does not save or refresh credentials and makes no changes to the proxy. It refuses redirects (including same-host redirects) and remote HTTP so a token cannot be forwarded into an unexpected login flow. Configure the canonical HTTPS URL. Loopback HTTP is allowed for local mock tests.

HTTP 302 usually means browser login is required. HTTP 401/403 may mean the token is expired, for another provider, or not permitted by application policy. HTML with HTTP 200 is not accepted as an API response. A network error may indicate DNS, TLS trust, timeout, or routing; install the appropriate CA rather than disabling verification.

Authentik proxy providers can accept provider-issued bearer JWTs or username/app-password authentication. This initial command supports bearer tokens only. Full device-code login like Klipbord requires verifying the issuer, public client ID, enabled grant, scopes, and whether the proxy accepts those issued tokens; it is not configured automatically here.

References: [Authentik header authentication](https://docs.goauthentik.io/add-secure-apps/providers/proxy/header_authentication/) and [machine-to-machine authentication](https://docs.goauthentik.io/add-secure-apps/providers/oauth2/machine_to_machine/).

## What still requires your camera network

After the read-only check, use the existing TTS preset speaker-comparison widget and its explicit sound confirmation. Repeat both orders, listen for clipped speech or gaps, and test Stop during preparation and playback. Then test speech interrupting AirPlay and confirm audible recovery. Check that configuration survives restart. Follow [the isolated backup/restore drill](verification.md) before considering a backup verified.

Remote API access does not give this development machine direct network access to the cameras or Lemonade. It can exercise CamSpeak's server-side routes once authorized; physical sound quality still requires a listener or suitable recording at the target.
