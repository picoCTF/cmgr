# Runtime Env Vars

- Namespace: cmgr/examples
- Type: custom
- Category: General Skills
- Points: 50

## Description

This challenge starts a simple web server that returns the `CMGR_USER_ID` and `CMGR_CUSTOM_VAR` environment variables provided securely at runtime.

## Details

The server at `{{ link("server", "/") }}` returns the environment variables.

This challenge was designed specifically to test the dynamic environment variable injection feature in `cmgr`.
