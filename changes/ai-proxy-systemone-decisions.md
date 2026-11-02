<!--
Copyright 2026 alibaba

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
-->

# Add System One and OpenAI Decisions routing to ai-proxy

`ai-proxy` now has a `typesafe` provider that serves both TypeSafe's native
`POST /v1/systemone` and the OpenAI `POST /v1/decisions` shape. The two wire
formats share a provider-neutral IR; the native route is forwarded unchanged,
and the OpenAI route is translated to System One and back.

The mock server gains deterministic `/v1/systemone` and `/v1/decisions`
handlers, and a new kind conformance case covers both routes plus local
rejection of malformed decision bodies. Native OpenAI upstreams and image
input on `/v1/decisions` remain out of scope and are documented as
limitations.
