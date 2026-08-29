# MeetWhen v2 Concierge Prototype — Product, UX, and Technical Specification

- **Status:** Proposed prototype specification
- **Related proposal:** [MeetWhen v2: Intent-first, agentic scheduling](../proposals/meetwhen-v2-agentic-scheduling.md)
- **Related issue:** [#53](https://github.com/Ceesaxp/meet-when/issues/53)
- **Primary surface:** Isolated concierge route `/c/{tenant}/{host}`
- **Prototype capability:** Read-only calendar availability; no new booking creation

## 1. Executive summary

MeetWhen currently provides two public scheduling surfaces:

- a host portal listing public meeting types at `/m/{tenant}/{host}`;
- direct booking links scoped to a meeting type at `/m/{tenant}/{host}/{template}`.

A meeting type is not merely a label. It is a scheduling policy that determines duration, calendars, required hosts, availability rules, buffers, notice, scheduling horizon, approval, conferencing, and other booking behavior. Direct booking links remain valuable because they select this policy in advance.

The MeetWhen v2 prototype adds a lightweight conversational concierge on an isolated `/c/{tenant}/{host}` route. The existing `/m/` host portal and direct booking links remain unchanged during testing. A visitor explains why they want to meet and, optionally, when they are generally available. The concierge:

1. detects the request language;
2. obtains the visitor timezone from the browser, visibly and correctably;
3. recognizes a returning visitor through an opaque first-party cookie when possible;
4. retrieves safe, tenant-scoped contact-history metadata;
5. matches the request to a small set of agent-discoverable meeting types;
6. selects the most appropriate type or asks one clarifying question;
7. queries MeetWhen's existing availability engine under that meeting type's rules;
8. presents several valid start times in a simple chat interface;
9. optionally lets the visitor inspect the same availability in a familiar calendar grid.

The portal does not disclose internal meeting-type names, routing metadata, or duration during classification. The prototype does not create bookings, hold slots, modify calendars, send invitations, or negotiate over email. Its purpose is to test whether intent, identity history, and richer meeting-type metadata can reliably route a visitor to useful availability.

## 2. Product thesis

> **The public portal should infer the appropriate scheduling policy from the visitor's request and relationship history, then show availability under that policy. Direct booking links continue to select the policy explicitly.**

The concierge belongs at the point of ambiguity. A direct link already encodes the meeting purpose; it should remain fast and familiar. The general portal does not know the purpose, so it benefits from conversational routing.

## 3. Goals

### 3.1 Product goals

- Let a visitor describe the purpose of a meeting in their own language.
- Select the most appropriate existing meeting type without forcing the visitor to navigate MeetWhen's internal taxonomy.
- Use exact-contact history as a routing prior when the visitor can be recognized or supplies an email.
- Show valid host availability calculated under the selected meeting type's existing rules.
- Preserve a familiar grid view as an optional, synchronized representation of availability.
- Keep direct meeting-type booking links unchanged.
- Produce enough routing and interaction evidence to evaluate whether the concept works.

### 3.2 Technical goals

- Add richer internal routing metadata to meeting types.
- Introduce a model-independent concierge orchestration layer.
- Add read-only v2 backend capabilities for routing context, contact history, and availability.
- Use a constrained classifier response containing only supplied meeting-type IDs.
- Add an opaque visitor-recognition cookie after a successful existing meeting request.
- Preserve deterministic calendar and policy enforcement in the existing backend.
- Keep the prototype deployable in the existing Go application while maintaining interfaces that can later be split into services.

## 4. Non-goals

The prototype will not:

- create a new booking through the concierge;
- hold or reserve a slot;
- modify any calendar;
- send invitations or meeting-request emails;
- cancel or reschedule bookings conversationally;
- connect to the visitor's calendar;
- perform multi-party scheduling;
- autonomously approve or reject requests;
- expose private or direct-link-only meeting types;
- learn or mutate routing policies automatically;
- replace the existing template booking page or host dashboard;
- treat a recognition cookie as authentication.

The existing booking flow remains operational and may set the recognition cookie after a successful request, but the new concierge itself is read-only with respect to bookings and calendars.

## 5. Terminology

### Meeting type

The existing `MeetingTemplate`. It defines a scheduling policy including duration, calendars, hosts, availability rules, buffers, notice, horizon, approval, conferencing, and questions.

### Routing metadata

Internal host-authored guidance explaining when a meeting type should or should not be selected by the concierge.

### Concierge

The public conversational experience and its orchestration layer.

### Classifier

A model adapter that converts visitor text and safe context into constrained, structured routing output.

### Recognized visitor

A browser presenting a valid `mw_visitor` cookie associated server-side with a tenant-scoped contact. This is a personalization signal, not verified identity.

### Scheduling backend

The deterministic MeetWhen services that resolve tenants, hosts, templates, contact history, and template-specific availability.

## 6. Product principles

### 6.1 Meeting types remain internal scheduling policies

The concierge selects a policy; it does not flatten all host availability into one generic calendar. Every availability query must include a valid meeting type.

### 6.2 Do not disclose internal taxonomy unnecessarily

The portal should speak in terms of the visitor's purpose rather than exposing template names, routing scores, or duration during classification. Exact start and end information can be disclosed later when a booking is actually created through the conventional flow.

### 6.3 Current intent dominates history

Previous meeting types help resolve ambiguity but must not override an explicit new purpose.

### 6.4 Ask the minimum useful clarification

High-confidence routing proceeds without announcing the internal decision. Medium-confidence routing asks one semantic question. Low-confidence routing asks the visitor to provide more context.

### 6.5 The model does not determine availability

The classifier may select from supplied meeting types and normalize time constraints. Only MeetWhen calculates and returns available slots.

### 6.6 Recognition is not authentication

A visitor cookie may influence routing and reduce repeated questions. It must never grant access to booking management, contact records, private templates, or calendar details.

### 6.7 Chat first, grid available

The prototype begins as a lightweight chat. Once availability exists, the visitor may inspect the same results in a conventional grid. Chat and grid share one state.

## 7. User experience

### 7.1 Entry point

The concierge is exposed at:

```text
/c/{tenant}/{host}
```

The existing conventional host portal remains at:

```text
/m/{tenant}/{host}
```

Direct links remain focused booking pages:

```text
/m/{tenant}/{host}/{template}
```

Private meeting types remain available only through direct links and are never supplied to the concierge classifier.

### 7.2 Initial portal

The initial experience should be intentionally small:

```text
Meet with Andrei

How can I help?

┌────────────────────────────────────────────┐
│ Tell me what you would like to discuss…    │
└────────────────────────────────────────────┘

                                      Send →

Times will be shown in Europe/Belgrade · Change
```

Below or behind a secondary action:

```text
Prefer to choose directly? View meeting options
```

This preserves access to the current public meeting-type list without making it the primary interface.

#### UI constraints

- Use a centered, narrow conversation column.
- Avoid a customer-support widget aesthetic.
- Do not use avatars unless later testing demonstrates value.
- Use plain message blocks and compact slot cards.
- Keep one primary text input fixed near the bottom on mobile.
- Allow keyboard submission while preserving multiline input through Shift+Enter.
- Show inferred timezone before any times are returned.
- Answer in the detected request language.

### 7.3 First-time anonymous visitor

Example:

```text
Visitor:
We are raising a seed round and would like to show Andrei the company.

Concierge:
Understood. When generally works for you?

Visitor:
Tuesday or Wednesday afternoon next week.

Concierge:
Andrei is available at:

[ Tuesday, 15:30 ]
[ Wednesday, 14:00 ]
[ Wednesday, 17:00 ]

[View calendar]
```

The portal does not disclose the selected template name or duration.

### 7.4 Recognized returning visitor

The browser sends `mw_visitor`. The middleware resolves tenant-scoped, summarized contact history.

Example:

```text
Visitor:
I would like to discuss the revised numbers and next steps.
```

Internal context may indicate that this contact previously used an investment follow-up meeting type. If current intent and history agree, the concierge proceeds without asking the visitor to identify the meeting type.

It should not display sensitive historical details on a potentially shared device. Avoid statements such as:

```text
You met Andrei on 12 August about Acme's revenue forecast.
```

A safe clarification is:

```text
Is this a follow-up to a previous company discussion, or a new topic?
```

### 7.5 Email-assisted recognition

If classification is ambiguous and identity history could help, ask for email:

```text
What email have you previously used with MeetWhen?
```

Do not require email when request semantics are already sufficient to classify the meeting. Email collection at this stage is for routing only; it does not create a booking.

### 7.6 Ambiguous request

```text
Visitor:
I want to speak to Andrei about Acme.

Concierge:
Is this an initial introduction, a company presentation, or a follow-up to a previous discussion?
```

The clarifying choices should use natural purpose language rather than internal template names.

### 7.7 Low-confidence or unsupported request

```text
Concierge:
Could you briefly say what you would like to discuss with Andrei?
```

If no agent-discoverable meeting type is suitable:

```text
I could not confidently match this request to an available meeting option.
You can review the public meeting options instead.

[View meeting options]
```

The model must not route to a private or excluded template.

### 7.8 Availability response

Slot responses should initially show start time only:

```text
Andrei is available at:

[ Tuesday, 15:30 ]
[ Wednesday, 14:00 ]
[ Wednesday, 17:00 ]

Times shown in Europe/Belgrade
[View calendar]
```

Use neutral wording: these times are valid under the selected host policy but are not known to match the visitor's own calendar unless they stated constraints.

If the visitor supplied constraints, grounded labels are allowed:

- `Matches Tuesday afternoon`
- `Earliest match`
- `Closest available alternative`

Avoid generic claims such as `Best for you`.

### 7.9 Grid view

The grid is a first-class synchronized view, not an error fallback.

- It displays availability for the already selected meeting type.
- It uses the same timezone and interpreted date range.
- Suggested slots may be highlighted.
- The visitor can switch back to chat without losing state.
- Clicking a different day may update the conversational context.
- The prototype does not submit a booking from either view.

For the read-only spike, clicking a slot may display:

```text
This time is currently available.
Booking from the concierge is not enabled in this prototype.
```

Optionally, a clearly labeled link may open the existing direct booking page for manual completion, but this is not required for prototype success.

### 7.10 Direct meeting-type links

Direct booking links remain unchanged. They continue to:

- bypass concierge routing;
- calculate availability using the linked meeting type;
- show the existing grid and booking form;
- create immediate or approval-pending bookings through the existing flow.

After a successful request, the existing flow may create or refresh the recognition cookie described in section 12.

## 8. Conversation state machine

```text
START
  ↓
WAITING_FOR_PURPOSE
  ↓
CLASSIFYING
  ├── high confidence → MEETING_TYPE_SELECTED
  ├── medium confidence → WAITING_FOR_CLARIFICATION
  ├── identity useful → WAITING_FOR_EMAIL
  └── no match → UNMATCHED

MEETING_TYPE_SELECTED
  ├── timing constraints available → FETCHING_AVAILABILITY
  └── no timing constraints → WAITING_FOR_TIMING

WAITING_FOR_TIMING
  ↓
FETCHING_AVAILABILITY
  ├── slots found → SHOWING_AVAILABILITY
  ├── no exact slots → SHOWING_ALTERNATIVES
  └── backend failure → AVAILABILITY_ERROR

SHOWING_AVAILABILITY
  ├── refine request → CLASSIFYING/FETCHING_AVAILABILITY
  ├── view grid → SHOWING_GRID
  └── end prototype → COMPLETE_READ_ONLY
```

The server owns state transitions. The browser should not be trusted to declare a selected template or recognized contact.

## 9. Architecture

```text
Browser
  │
  │ public concierge messages
  ▼
Concierge HTTP handler
  ▼
Concierge orchestrator
  ├── visitor resolver
  ├── contact-history summarizer
  ├── routing-context provider
  ├── classifier adapter
  ├── constraint normalizer/validator
  └── availability client
          ▼
MeetWhen v2 read API / service interface
  ├── tenant + host resolution
  ├── agent-discoverable meeting types
  ├── tenant-scoped contact history
  └── AvailabilityService.GetAvailableSlots
          ▼
Calendar providers + MeetWhen database
```

### 9.1 Prototype deployment shape

Keep the concierge in the existing Go application for the prototype. Add clean interfaces so components can later be separated:

```go
type Classifier interface {
    Classify(context.Context, ClassificationInput) (*ClassificationResult, error)
}

type SchedulingBackend interface {
    RoutingContext(context.Context, RoutingContextInput) (*RoutingContext, error)
    ContactHistory(context.Context, ContactHistoryInput) (*ContactHistory, error)
    Availability(context.Context, AvailabilityInput) ([]models.TimeSlot, error)
}
```

The first `SchedulingBackend` implementation may call existing services in-process. A later HTTP implementation can call a separately deployed v2 API without changing orchestration behavior.

Do not introduce a new microservice merely to prove the concept.

### 9.2 Suggested package layout

```text
internal/concierge/
  orchestrator.go
  conversation.go
  classifier.go
  constraints.go
  routing.go
  visitor.go
  types.go

internal/handlers/
  concierge.go
  api_v2.go

internal/services/
  routing_context.go

internal/repository/
  visitor_identity.go

migrations/
  015_add_meeting_template_routing_metadata.up.sql
  016_add_visitor_identity.up.sql

templates/pages/
  public_host_concierge.html

templates/partials/
  concierge_messages.html
  concierge_slots.html
  concierge_grid.html

static/js/
  concierge.js
```

Exact migration numbers should be adjusted if another migration lands first.

## 10. Meeting-type routing metadata

### 10.1 Required fields

Add to `meeting_templates`:

```text
routing_description TEXT NOT NULL DEFAULT ''
routing_examples JSON/JSONB NOT NULL DEFAULT '[]'
routing_exclusions JSON/JSONB NOT NULL DEFAULT '[]'
agent_discoverable BOOLEAN NOT NULL DEFAULT FALSE
routing_priority INTEGER NOT NULL DEFAULT 0
```

SQLite and PostgreSQL migrations must remain aligned.

### 10.2 Semantics

### `routing_description`

Private host-authored instructions explaining when to use the meeting type.

Example:

```text
Use when a founder or company representative wants to present a startup
or fundraising opportunity for possible DSV investment. This is suitable
for a first substantive review.
```

### `routing_examples`

Positive example visitor requests in one or more languages.

### `routing_exclusions`

Requests that may look similar but should use another type or remain unmatched.

### `agent_discoverable`

Whether the concierge may consider the type. This is independent of whether the type is listed publicly.

### `routing_priority`

A deterministic host preference used only as a tie-breaker or weak prior. It must not override a clearly better semantic match.

### 10.3 Visibility matrix

| Template state | Listed on conventional portal | Available to concierge | Direct link |
|---|---:|---:|---:|
| Public + agent discoverable | Yes | Yes | Yes |
| Public + not agent discoverable | Yes | No | Yes |
| Private + not agent discoverable | No | No | Yes |
| Private + agent discoverable | No by default | No in prototype | Yes |

The prototype must not route to private templates even if incorrectly configured as discoverable. Supporting non-listed but agent-routable types is a later explicit product decision.

### 10.4 Host administration UX

Add an **Agent routing** section to the meeting-template form:

```text
Agent routing

[ ] Allow the public concierge to select this meeting type

When should this type be used?
[ multiline routing description ]

Example requests
[ one example per line ]

Do not use for
[ one exclusion per line ]

Routing priority
[ number; advanced setting ]
```

Routing metadata is never rendered on public booking pages.

## 11. Classification and orchestration

### 11.1 Inputs

The classifier receives only:

- current visitor message;
- bounded prior conversation summary;
- browser/request language hints;
- visitor timezone;
- safe, summarized contact history if available;
- eligible meeting-type IDs and routing metadata;
- current normalized time constraints;
- current date and timezone for relative-date interpretation.

Do not provide:

- raw calendar events;
- private templates;
- booking tokens;
- contact notes;
- other attendees;
- complete historical transcripts;
- calendar credentials.

### 11.2 Structured output

```json
{
  "language": "en",
  "request_summary": "Follow-up on an investment discussion",
  "candidates": [
    {
      "template_id": "investment-follow-up",
      "confidence": 0.91,
      "reason_codes": [
        "FOLLOW_UP_LANGUAGE",
        "CONTACT_HISTORY_MATCH"
      ]
    },
    {
      "template_id": "project-pitch",
      "confidence": 0.58,
      "reason_codes": [
        "INVESTMENT_TOPIC"
      ]
    }
  ],
  "selected_template_id": "investment-follow-up",
  "needs_clarification": false,
  "clarifying_question": null,
  "time_constraints": {
    "date_from": "2026-09-01",
    "date_to": "2026-09-07",
    "weekdays": [2, 3],
    "time_after": "13:00",
    "time_before": "18:00",
    "timezone": "Europe/Belgrade"
  }
}
```

Weekdays use a documented numeric convention consistent with MeetWhen models.

### 11.3 Validation

The orchestrator must:

- reject template IDs not present in the supplied candidate set;
- clamp confidence to `[0, 1]`;
- validate IANA timezone names;
- normalize and bound date ranges;
- enforce the template's scheduling horizon;
- reject malformed times;
- cap the number of candidates;
- cap classifier retries;
- treat all model explanations as untrusted presentation text;
- generate public slot responses from backend data, not model output.

### 11.4 Confidence policy

Initial configurable thresholds:

```text
High confidence:   top score >= 0.80 and lead over second >= 0.15
Medium confidence: top score >= 0.55
Low confidence:    otherwise
```

- **High:** select internally and proceed.
- **Medium:** ask one clarifying question.
- **Low:** ask for more context or offer conventional meeting options.

Thresholds are prototype defaults and should be evaluated, not treated as permanent product truth.

### 11.5 Contact-history prior

Evidence strength, highest first:

1. exact email's previous bookings with this tenant/host;
2. most recent meeting type used by that contact;
3. repeated meeting-type pattern;
4. explicit relationship metadata, if later introduced;
5. organization/domain history;
6. email domain alone.

Current request semantics must dominate. Domain inference alone is weak and should not auto-select a type.

### 11.6 Language handling

- Detect language primarily from visitor text.
- Use `Accept-Language` only as a weak initial hint.
- Use prior contact language only when the current request does not make language clear.
- Reply in the detected request language.
- Keep classifier intent and reason codes language-independent.
- Do not expose raw Serbian/Cyrillic labels in internal English-language administration surfaces unless intentionally entered as routing examples.

## 12. Visitor recognition

### 12.1 Cookie

After a successful request through the existing booking flow, set:

```http
Set-Cookie: mw_visitor=<256-bit-random-token>;
            Path=/;
            Max-Age=15552000;
            HttpOnly;
            Secure;
            SameSite=Lax
```

For local HTTP development, `Secure` may be disabled by environment-aware cookie configuration, matching existing session-cookie behavior.

The cookie contains only an opaque random token.

### 12.2 Storage

```text
visitor_identities
- id
- token_hash UNIQUE
- created_at
- last_seen_at
- expires_at
- revoked_at NULL

visitor_contact_links
- visitor_id
- tenant_id
- contact_id
- source
- first_seen_at
- last_confirmed_at
- PRIMARY KEY (visitor_id, tenant_id, contact_id)
```

Store a cryptographic hash of the token, not the raw cookie value.

### 12.3 Lifecycle

On successful existing meeting request:

1. normalize the submitted email;
2. upsert/find the tenant contact through the existing contact pipeline;
3. resolve the current visitor cookie if valid, otherwise generate one;
4. associate the visitor identity with the tenant-scoped contact;
5. refresh `last_confirmed_at`, `last_seen_at`, and expiry;
6. send the cookie in the response that leads to the booking status page.

On concierge portal load:

1. read and hash `mw_visitor`;
2. resolve an active, unexpired visitor identity;
3. retrieve links only for the current tenant;
4. produce safe contact-history summaries;
5. refresh `last_seen_at` asynchronously or through a bounded write.

### 12.4 Security boundary

Recognition must not authorize:

- reading an existing booking;
- cancellation or rescheduling;
- contact-detail access;
- private template discovery;
- calendar access;
- host actions.

Existing booking tokens and authenticated host sessions remain authoritative.

### 12.5 Shared-device behavior

Do not display the recognized email or detailed prior meeting history automatically. Provide a later **Forget this browser** action that revokes the server token and clears the cookie.

A future production release should determine whether explicit opt-in is required for this functional personalization cookie in applicable jurisdictions. The prototype should include clear device-level language and make recognition easy to clear.

## 13. Read-only v2 backend API

The browser should call the concierge endpoint, not internal routing APIs directly. Routing metadata and contact history are service-side information.

### 13.1 Public host context

```http
GET /api/v2/public/{tenant}/{host}
```

Response:

```json
{
  "host": {
    "name": "Andrei Popov",
    "timezone": "Europe/Belgrade"
  },
  "concierge_enabled": true
}
```

No routing metadata or meeting-type IDs are returned.

### 13.2 Concierge message

```http
POST /api/v2/public/{tenant}/{host}/concierge/messages
Content-Type: application/json
```

Request:

```json
{
  "conversation_id": null,
  "message": "We are raising a seed round and would like to present the company.",
  "timezone": "Europe/Belgrade",
  "email": null
}
```

Response when clarification is needed:

```json
{
  "conversation_id": "conv_...",
  "state": "waiting_for_clarification",
  "language": "en",
  "assistant_message": "Is this an initial presentation or a follow-up to a previous discussion?",
  "slots": []
}
```

Response with availability:

```json
{
  "conversation_id": "conv_...",
  "state": "showing_availability",
  "language": "en",
  "assistant_message": "Andrei is available at:",
  "timezone": "Europe/Belgrade",
  "slots": [
    {"start": "2026-09-01T15:30:00+02:00"},
    {"start": "2026-09-02T14:00:00+02:00"},
    {"start": "2026-09-02T17:00:00+02:00"}
  ],
  "calendar_available": true
}
```

Do not return selected internal template IDs to the browser unless required for a later trusted transition. For the prototype, retain them server-side in conversation state.

### 13.3 Availability backend capability

Internal interface or service-authenticated endpoint:

```http
POST /api/v2/internal/availability
```

```json
{
  "tenant_id": "...",
  "host_id": "...",
  "template_id": "...",
  "timezone": "Europe/Belgrade",
  "date_from": "2026-09-01",
  "date_to": "2026-09-07"
}
```

The backend:

- verifies tenant, host, and template relationship;
- verifies the template is active, public, and agent-discoverable;
- selects an allowed template duration internally;
- calls `AvailabilityService.GetAvailableSlots`;
- applies normalized visitor constraints;
- returns bounded valid slots.

The public presentation may omit slot end time, but internal calculations must use it.

### 13.4 Routing-context capability

Internal interface or service-authenticated endpoint returns only active, public, agent-discoverable templates and their routing metadata.

### 13.5 Contact-history capability

Internal interface or service-authenticated endpoint accepts current tenant, host, and normalized email/contact identity, then returns a summary:

```json
{
  "known_contact": true,
  "meeting_count": 3,
  "previous_templates": [
    {
      "template_id": "investment-follow-up",
      "count": 2,
      "most_recent": "2026-08-10T14:00:00Z"
    }
  ],
  "preferred_language": "en",
  "last_timezone": "Europe/Belgrade"
}
```

Do not return booking tokens, answers, conference links, or unrelated contact fields.

## 14. Conversation persistence

For the prototype, conversation state may be stored server-side with a short expiry.

Suggested fields:

```text
concierge_conversations
- id
- tenant_id
- host_id
- visitor_identity_id NULL
- language
- timezone
- state
- selected_template_id NULL
- normalized_constraints JSON
- bounded_summary TEXT
- created_at
- updated_at
- expires_at
```

Do not store unlimited raw transcripts by default. For evaluation, store redacted or explicitly bounded interaction events separately.

A memory-backed store is acceptable for an initial local spike. A database table is required before multi-instance deployment.

## 15. Availability selection

The prototype's shortlist algorithm is deterministic after classification:

1. fetch slots under the selected template;
2. apply explicit visitor hard constraints;
3. rank exact matches before relaxed alternatives;
4. prefer earlier valid slots unless host policy says otherwise;
5. avoid returning three adjacent slots when a diverse set is available;
6. return approximately three options;
7. attach machine-readable reason codes;
8. generate visitor wording from those reason codes.

Possible reason codes:

```text
EXACT_TIME_WINDOW_MATCH
EXACT_WEEKDAY_MATCH
EARLIEST_MATCH
HOST_PREFERRED_WINDOW
CLOSEST_ALTERNATIVE
RELAXED_DAY_CONSTRAINT
RELAXED_TIME_CONSTRAINT
```

The model must not invent ranking reasons.

## 16. Frontend implementation

### 16.1 Rendering approach

Continue using server-rendered templates and HTMX where practical. A small amount of JavaScript may manage:

- browser timezone detection;
- input behavior;
- optimistic pending state;
- chat/grid view switching;
- scrolling to the latest message.

Do not introduce a new SPA framework for the prototype.

### 16.2 Components

#### Conversation transcript

- visitor messages aligned consistently;
- assistant responses in plain blocks;
- semantic live region for new assistant content;
- bounded width for readability.

#### Composer

- textarea with accessible label;
- send button;
- disabled/loading state;
- visible error state;
- optional email field only when requested by the server.

#### Slot cards

- large click/tap targets;
- locale-aware date/time formatting;
- timezone displayed once near the group;
- optional grounded reason label;
- no internal meeting-type name or duration.

#### Calendar grid

Reuse or adapt the existing slot grid, driven by the concierge's selected meeting type and constraints. Do not duplicate availability calculations in JavaScript.

### 16.3 Accessibility

- All interactions keyboard accessible.
- New assistant messages announced through `aria-live="polite"`.
- Loading state exposed through `aria-busy`.
- Slot cards use buttons, not clickable divs.
- Grid preserves existing semantic controls.
- Color is not the sole indicator of suggested slots.
- Focus moves predictably after submitting a message or switching views.
- Respect `prefers-reduced-motion`.

### 16.4 Responsive behavior

- Single-column chat on mobile.
- Composer remains reachable above mobile browser controls.
- Grid opens as a separate view or panel rather than beside chat.
- Desktop may show the conversation and grid side by side after availability is loaded.

## 17. Model provider strategy

Use a provider interface. The prototype should support:

- a deterministic fake classifier for tests and demos;
- one structured-output-capable model adapter for live testing.

Configuration should be environment-based and must not hard-code provider credentials.

The fake classifier should support scripted outcomes:

- high-confidence selection;
- medium-confidence clarification;
- no match;
- malformed model output;
- multilingual response.

If the model provider is unavailable, offer the conventional public meeting options rather than failing the entire portal.

## 18. Security and privacy

- Treat visitor text as untrusted input.
- Supply only enumerated eligible template IDs to the classifier.
- Validate all classifier output server-side.
- Never expose raw routing metadata publicly.
- Never expose raw calendar events to the classifier or browser.
- Return free slots only.
- Scope contact history to current tenant and host.
- Normalize email before lookup.
- Rate limit concierge messages by IP, visitor token, and host.
- Cap message size, conversation length, date range, candidate count, and model retries.
- Hash visitor tokens at rest.
- Do not log raw cookies or booking tokens.
- Redact or minimize email and request text in telemetry.
- Preserve existing tenant isolation.
- Add audit events for routing-context access and recognized-visitor resolution without logging sensitive content.

## 19. Failure and fallback behavior

### Classifier unavailable

```text
I cannot interpret requests right now. You can still review the available meeting options.

[View meeting options]
```

### Calendar provider unavailable

```text
I cannot check Andrei's availability right now. Please try again shortly or review the meeting options.
```

Do not treat a calendar error as an empty calendar.

### No availability

```text
I could not find availability matching that request. Would you like to try a wider date range or different days?
```

### Invalid timezone

Fall back to UTC only internally; ask the visitor to choose a timezone before presenting times.

### Unrecognized or expired visitor cookie

Proceed anonymously and optionally replace the token after the next successful conventional request.

## 20. Instrumentation and evaluation

Record structured events:

```text
concierge.started
concierge.language_detected
concierge.visitor_recognized
concierge.email_supplied
concierge.classified
concierge.clarification_requested
concierge.template_selected
concierge.availability_requested
concierge.availability_shown
concierge.grid_opened
concierge.unmatched
concierge.fallback_used
concierge.error
```

Recommended fields:

- tenant and host IDs;
- conversation ID;
- recognized-versus-anonymous state;
- candidate template IDs and scores;
- selected template ID;
- reason codes;
- whether contact history changed ranking;
- clarification count;
- detected language;
- broad constraint categories;
- slot count;
- latency by stage;
- fallback/error category.

Avoid recording raw visitor text or email in analytics unless explicitly required for a controlled evaluation dataset.

## 21. Evaluation dataset

Create 30–50 reviewed scenarios covering:

- new investment pitches;
- investment follow-ups;
- portfolio-company discussions;
- general introductions;
- sales approaches;
- known and unknown contacts;
- English, Russian, and other likely languages;
- explicit timing constraints;
- vague requests;
- conflicting current intent and history;
- prompt-injection attempts;
- requests with no suitable public meeting type.

Each scenario should specify:

```text
input message
language
recognized-contact summary, if any
eligible meeting types
expected selected type or clarification
expected constraint normalization
```

Primary evaluation metrics:

- top-1 meeting-type accuracy;
- top-3 candidate recall;
- unnecessary clarification rate;
- unsafe/private-template selection rate;
- contact-history benefit versus semantic-only routing;
- availability validity;
- fallback rate;
- median response latency.

## 22. Prototype acceptance criteria

The prototype is complete when all of the following are demonstrated with real MeetWhen availability data:

1. `/c/{tenant}/{host}` renders a lightweight chat interface.
2. The conventional `/m/{tenant}/{host}` portal and direct meeting-type links remain unchanged.
3. Host administrators can configure routing description, examples, exclusions, discoverability, and priority.
4. The concierge detects and responds in the request language.
5. Browser timezone is inferred, displayed, and correctable.
6. The classifier receives only eligible public meeting types and returns constrained IDs.
7. A high-confidence request selects the correct internal meeting type without exposing its name or duration.
8. A medium-confidence request produces one useful clarification.
9. A recognized visitor can contribute tenant-scoped contact-history metadata to routing.
10. Contact history never overrides an explicit conflicting current request.
11. Availability is calculated through `AvailabilityService.GetAvailableSlots` under the selected meeting type.
12. The portal displays several valid start times without exposing calendar events.
13. The same availability can be inspected in a familiar grid.
14. No concierge operation creates a booking, holds a slot, or modifies a calendar.
15. Classifier and calendar failures fall back safely.
16. Visitor recognition does not grant booking or contact access.
17. Unit and handler tests cover routing validation, cookie lifecycle, tenant scope, confidence behavior, and read-only guarantees.
18. The complete Go test suite passes.

## 23. Recommended implementation sequence

### Phase A — Routing metadata

- Add PostgreSQL and SQLite migrations.
- Extend `MeetingTemplate` model and repository queries.
- Add template-form fields and tests.

### Phase B — Visitor recognition

- Add visitor identity/link tables and repository.
- Set or refresh `mw_visitor` after successful existing requests.
- Resolve tenant-scoped history on portal visits.
- Add revocation/forget capability if exposed in the prototype.

### Phase C — Read-only backend capabilities

- Add routing-context service.
- Add safe contact-history summarization.
- Add bounded template-specific availability method.
- Add v2 handler contracts and tests.

### Phase D — Classifier and orchestrator

- Define structured classifier interface and fake provider.
- Add output validation and confidence policy.
- Add conversation state and safe fallback.
- Add one live provider adapter.

### Phase E — Lightweight chat and grid

- Add the concierge interface on the isolated `/c/{tenant}/{host}` route.
- Leave the existing `/m/` portal and direct booking routes unchanged.
- Add HTMX/JS conversation interactions.
- Render slots and grid from server responses.
- Preserve conventional meeting-option access.

### Phase F — Evaluation

- Build reviewed scenario set.
- Run classification and security cases.
- Dogfood against real calendar availability.
- Decide whether to proceed to slot selection, holds, and booking creation.

## 24. Open decisions

1. Which structured-output model provider should power the live prototype?
2. Should recognition be opt-in after a successful request or enabled with a clear functional-cookie notice?
3. How long should `mw_visitor` remain valid?
4. Should a recognized visitor's previously supplied email ever be prefilled, or only used silently as a routing prior?
5. Should the prototype allow a slot click to deep-link into the existing booking page?
6. What default date range should be searched when the visitor provides no timing preference?
7. Should host routing priority be visible as an advanced numeric field or represented through ordered meeting types?
8. Should conversation state use a database table immediately or begin with an in-memory store?
9. Which languages must be included in the first reviewed evaluation set?

## 25. Decision gate after prototype

Proceed to writable concierge scheduling only if the prototype demonstrates:

- reliable meeting-type selection;
- useful clarification behavior;
- materially improved routing from recognized-contact history;
- valid availability under the correct meeting-type policy;
- acceptable latency and fallback rates;
- no private-template, tenant-isolation, or calendar-disclosure failures.

Only then should MeetWhen add slot holds, explicit selection, idempotent booking confirmation, invitation creation, and conversational rescheduling.
