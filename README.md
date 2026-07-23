# json-schema-tools

Composable **JSON Schema validation** nodes for the [Axiom](https://axiomide.com)
marketplace, wrapping the Apache-2.0
[`santhosh-tekuri/jsonschema/v6`](https://github.com/santhosh-tekuri/jsonschema)
library — a validator that passes the official JSON Schema Test Suite for
Drafts 4, 6, 7, 2019-09 and 2020-12.

Every node is **stateless, offline, and deterministic**: it reads only its
input, performs no I/O, and returns the same result for the same input.

## Use it from your agent or app

Every node in this package is a **live, auto-scaling API endpoint** on the
[Axiom](https://axiomide.com) marketplace — call it from an AI agent or your own
code, with nothing to self-host.

**📦 See it on the marketplace:**
https://dev.axiomide.com/marketplace/christiangeorgelucas/json-schema-tools@0.4.0

**Hook it up to an AI agent (MCP).** Add Axiom's hosted MCP server to any MCP
client and every node becomes a typed tool your agent can call — search the
catalog, inspect a schema, and invoke it directly.

```bash
# Claude Code
claude mcp add --transport http axiom https://api.axiomide.com/mcp \
  --header "Authorization: Bearer $AXIOM_API_KEY"
```

Claude Desktop, Cursor, or any config-based client:

```json
{
  "mcpServers": {
    "axiom": {
      "type": "http",
      "url": "https://api.axiomide.com/mcp",
      "headers": { "Authorization": "Bearer YOUR_AXIOM_API_KEY" }
    }
  }
}
```

**Call it from the CLI.**

```bash
axiom invoke christiangeorgelucas/json-schema-tools/Validate --input '{ ... }'
```

**Call it over HTTP.**

```bash
curl -X POST https://api.axiomide.com/invocations/v1/nodes/christiangeorgelucas/json-schema-tools/0.4.0/Validate \
  -H "Authorization: Bearer $AXIOM_API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{ ... }'
```

> Input/output schema for each node is on the marketplace page above, or via
> `axiom inspect node christiangeorgelucas/json-schema-tools/Validate`.

### Get started free

Install the CLI:

```bash
# macOS / Linux — Homebrew
brew install axiomide/tap/axiom

# macOS / Linux — install script
curl -fsSL https://raw.githubusercontent.com/AxiomIDE/axiom-releases/main/install.sh | sh
```

**Windows:** download the `windows/amd64` `.zip` from the
[releases page](https://github.com/AxiomIDE/axiom-releases/releases), unzip it,
and put `axiom.exe` on your `PATH`.

Then `axiom version` to verify, `axiom login` (GitHub or Google) to authenticate,
and create an API key under **Console → API Keys**. Docs and sign-up at
**[axiomide.com](https://axiomide.com)**.

## Nodes

| Node | Purpose |
|------|---------|
| **Validate** | Validate one JSON instance against one JSON Schema. Returns `valid` plus structured `errors` (each with the instance path, the failing schema keyword path, and a message). |
| **ValidateMany** | Validate many instances against a single schema in one call (the schema is compiled once). Returns one result per instance, in input order. |
| **CheckSchema** | Check that a document is itself a well-formed, compilable JSON Schema for the detected/selected draft, with structured reasons when it is not. |
| **DescribeSchema** | Compile a schema and report structural metadata about the schema itself — no instance needed: effective draft, root JSON types, `title`/`description`/`deprecated`/`readOnly`/`writeOnly` annotations, `required` property names, and top-level `properties` with their own types. A root-level `$ref` is resolved through one level. Boolean schemas (`true`/`false`) are reported via `is_boolean_schema`. |

All four take the schema (and, where applicable, instance/instances) as
**JSON text** (strings), so they compose with any node that produces JSON.
`DescribeSchema` shares its input message (`CheckSchemaRequest`) with
`CheckSchema` — both just need a schema and an optional draft.

### Draft selection

The draft is auto-detected from the schema's `$schema` field. For schemas that
omit it, the optional `draft` field selects a default — one of `"4"`, `"6"`,
`"7"`, `"2019"`, `"2020"` (default `"2020"`, i.e. Draft 2020-12).

### Format assertion

By default `format` keywords (`email`, `date-time`, `uri`, `ipv4`, …) are
treated as annotations and do not cause failure — the JSON Schema default. Set
`assert_formats: true` on `Validate`/`ValidateMany` to assert them.

## Safety

- **No external reference resolution.** A `$ref` to a `file://`, `http://`, or
  `https://` URL is rejected with a structured error — it is never fetched.
  Only references internal to the supplied schema document are resolved. This
  removes the SSRF / local-file-read surface a permissive validator would
  otherwise expose.
- **Linear-time regexes.** `pattern` and `format` matching use Go's RE2 engine,
  which has no catastrophic-backtracking (ReDoS) failure mode.
- **Bounded input.** Oversized schemas/instances are rejected with a structured
  error rather than being parsed.

## Error contract

Malformed input never crashes a node. When a request cannot be processed at all
(invalid JSON, a rejected external `$ref`, an uncompilable schema, an unknown
`draft`, or input over the size limit), the response's `error` field is set and
`valid` is false. When validation runs, `valid` reports the outcome and `errors`
explains any failures.

## Example

`Validate` input:

```json
{
  "schema": "{\"type\":\"object\",\"required\":[\"email\"],\"properties\":{\"email\":{\"type\":\"string\",\"format\":\"email\"}}}",
  "instance": "{\"email\":\"not-an-email\"}",
  "assert_formats": true
}
```

Output:

```json
{
  "errors": [
    { "instancePath": "/email", "keywordPath": "/properties/email/format",
      "message": "'not-an-email' is not valid email: missing @" }
  ]
}
```

## Development

```bash
axiom validate --json      # static checks
axiom test                 # unit + independent-oracle + security tests
axiom dev --port 8091 --socket /tmp/jst-axiom.sock   # local server
```

## License

MIT — see [LICENSE](./LICENSE). Copyright (c) 2026 Christian George Lucas.
Built for the Axiom marketplace.
