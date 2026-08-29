# MeetWhen v2: Intent-first, agentic scheduling

- **Status:** Proposal
- **Audience:** Product and engineering
- **Scope:** Public scheduling experience, scheduling API, and agent integrations

## Summary

MeetWhen v1 asks guests to operate scheduling software: choose a meeting type, inspect a calendar, select a slot, and complete a form.

MeetWhen v2 should instead ask a simpler question:

> **When would you like to meet?**

A guest can describe intent in ordinary language—"next week, Tuesday or Wednesday afternoon, about 45 minutes to discuss our pitch"—and MeetWhen proposes a small number of valid, ranked options. The guest can refine the request conversationally, select an option, and confirm or request the meeting.

The product thesis is:

> **MeetWhen v1 publishes available slots. MeetWhen v2 conducts the scheduling conversation.**

This should not be a replacement of the scheduling backend. The current Go application already provides the difficult deterministic capabilities: calendar integration, working hours, buffers, pooled hosts, templates, approval workflows, booking lifecycle, conferencing, contacts, notifications, cancellation, and rescheduling. V2 should preserve that backend as the scheduling authority and introduce an intent-driven experience above it.

## Why this direction

A conventional booking grid is efficient when the guest already understands the meeting type, duration, timezone, and acceptable dates. It is less effective when the request is approximate or constrained:

- "Tomorrow after 3."
- "Any morning next week except Thursday."
- "As soon as possible, but we need 45 minutes."
- "Find a time when both Andrei and Alexey are available."
- "Move this later, but keep it on Wednesday."

Calendly-style interfaces force the guest to translate intent into repeated filtering and calendar navigation. MeetWhen can perform that translation and offer a small number of sensible proposals.

The opportunity is not merely to add a chatbot to a calendar. A conversational skin alone would often be slower than selecting a slot. The useful differentiation is **policy-aware scheduling agency**:

- understand approximate and ambiguous constraints;
- infer an appropriate meeting type and duration;
- rank valid options rather than expose every slot;
- explain compromises when no exact match exists;
- respect host preferences beyond free/busy data;
- request approval only when required;
- preserve context during refinement, cancellation, and rescheduling;
- eventually coordinate several participants or agents.

## Product principles

### 1. Intent first, calendar second

The primary public interface should begin with a natural-language request. A conventional calendar remains available through **Show all times** for guests who prefer direct control or when interpretation fails.

### 2. Deterministic scheduling authority

The language model may interpret requests and formulate responses, but it must never independently determine that a slot is available. Availability, policy enforcement, slot validation, and booking remain deterministic backend operations.

### 3. Proposals, not slot dumps

The default response should contain approximately three ranked choices, each valid at the time it was generated. More options can be requested explicitly.

### 4. Progressive disclosure

Ask only for information needed at the current step. Do not begin with a long booking form. Gather identity, email, agenda, guests, and custom template questions once a suitable proposal has been selected.

### 5. Privacy by construction

The public experience must expose only permitted openings—not calendar titles, event details, unavailable periods, or unnecessary information about host routines.

### 6. Graceful fallback

Natural-language scheduling should enhance the existing flow, not make booking dependent on model availability. The standard calendar UI must remain usable.

## Example guest journey

### Initial request

```text
When would you like to meet Andrei?

> Next week, ideally Tuesday or Wednesday afternoon. We need about
> 45 minutes to discuss Realytics.
```

### Proposal

```text
I found three good options in your timezone:

1. Tuesday, 15:30–16:15 — earliest match
2. Wednesday, 14:00–14:45
3. Wednesday, 17:00–17:45

Andrei normally uses a 45-minute pitch session for this type of meeting.
Which works best?
```

### Refinement

```text
> Could we make it later on Wednesday?

The latest suitable opening is Wednesday, 17:00–17:45.
I can also offer Thursday at 18:00 if Wednesday is not essential.
```

### Confirmation

```text
Great. I will propose Wednesday at 17:00 CEST.
What email should receive the invitation?
```

If approval is required:

```text
Your proposal has been sent to Andrei. I will email you when it is
accepted or if an alternative is suggested.
```

If no exact match exists:

```text
Andrei has no 45-minute opening Tuesday afternoon. I can offer:

- Tuesday at 17:30
- Wednesday at 14:00
- a shorter 30-minute call Tuesday at 15:30
```

## Recommended interface

The experience should feel like a calm scheduling concierge rather than a customer-support chat widget.

Suggested first screen:

```text
MeetWhen

When can I meet with Andrei?

┌────────────────────────────────────────────┐
│ Next week, Tuesday or Wednesday afternoon, │
│ about 45 minutes to discuss our pitch.     │
└────────────────────────────────────────────┘

                    Find a time →

Examples:
"Tomorrow after 3"
"Any morning next week"
"ASAP, 30 minutes"
```

The same surface should transform into proposal cards and refinement controls. Chat bubbles are not required. The interaction can remain structured and visually concise while accepting conversational input.

## Two agent surfaces

### Public guest concierge

A public, tightly constrained agent that can:

- discover permitted meeting types;
- interpret date, time, timezone, duration, purpose, and flexibility;
- request ranked availability proposals;
- collect guest identity and required template answers;
- create immediate or approval-pending bookings;
- reschedule or cancel using a secure booking token.

It must not have general access to host calendars or administrative operations.

### Authenticated host agent

An authenticated agent that can eventually handle requests such as:

- "Find 45 minutes with Tony next week."
- "Approve the Realytics request."
- "Give Sid three alternatives."
- "Move tomorrow's investor calls later."
- "Protect Friday afternoon for deep work."
- "Require approval for contacts I have not met before."

The host agent can be exposed through the web application, native clients, Hermes, an MCP server, or other authorized assistants.

## Architecture

```text
Guest request
    ↓
Intent extraction
    - preferred dates and times
    - timezone
    - duration
    - meeting purpose
    - flexibility
    - participants
    ↓
MeetWhen public scheduling API
    ↓
Deterministic policy and availability engine
    ↓
Ranked valid proposals
    ↓
Natural-language or structured presentation
    ↓
Guest selection
    ↓
Final conflict check + booking/approval workflow
```

### Responsibility boundary

The agent layer may:

- parse and normalize natural-language constraints;
- ask clarifying questions;
- select from permitted meeting templates;
- request candidate slots;
- describe ranked proposals and compromises.

The backend must:

- authorize every operation;
- calculate actual availability;
- enforce working hours, notice, buffers, and scheduling horizon;
- validate meeting duration and template rules;
- protect tenant boundaries;
- hold and revalidate slots;
- create calendar events and conferencing links;
- send notifications;
- maintain an audit trail.

## Proposed public API

The current `/api/v1` primarily serves authenticated native clients and booking administration. Public scheduling is currently implemented through HTML and HTMX handlers. V2 should add a JSON-first public API without removing the existing pages.

Illustrative endpoints:

```text
GET  /api/v2/public/{tenant}/{host}
GET  /api/v2/public/{tenant}/{host}/meeting-types
POST /api/v2/public/{tenant}/{host}/proposals
POST /api/v2/public/proposals/{id}/refine
POST /api/v2/public/proposals/{id}/hold
POST /api/v2/public/proposals/{id}/confirm
GET  /api/v2/public/bookings/{token}
POST /api/v2/public/bookings/{token}/reschedule
POST /api/v2/public/bookings/{token}/cancel
```

Example proposal request:

```json
{
  "meeting_type": "pitch-45",
  "timezone": "Europe/Belgrade",
  "constraints": {
    "date_from": "2026-09-01",
    "date_to": "2026-09-07",
    "preferred_weekdays": ["tuesday", "wednesday"],
    "time_window": {"after": "13:00", "before": "18:00"},
    "duration_minutes": 45,
    "flexibility": "nearby"
  },
  "limit": 3
}
```

Example response:

```json
{
  "proposal_id": "prop_...",
  "expires_at": "2026-08-29T15:10:00Z",
  "options": [
    {
      "option_id": "opt_1",
      "start": "2026-09-01T15:30:00+02:00",
      "end": "2026-09-01T16:15:00+02:00",
      "rank": 1,
      "reason": "earliest exact match"
    }
  ],
  "compromises": []
}
```

The API should accept structured constraints. Natural-language parsing belongs in the agent layer so the scheduling core remains model-independent and testable.

## Proposal and hold state

A proposal is not yet a booking. A small amount of server-side state is useful for:

- preserving the interpreted constraints;
- supporting refinement without starting over;
- auditing what was shown to the guest;
- expiring stale options;
- rate limiting abusive clients.

A short-lived hold may reduce race conditions after selection. Confirmation must still perform a final authoritative availability check and use an idempotency key.

Potential entities:

- `scheduling_conversations`
- `scheduling_proposals`
- `proposal_options`
- `slot_holds`

The MVP can avoid storing full message transcripts. It may store only normalized constraints, proposal metadata, selected option, and necessary audit information.

## Ranking and policy

Ranking should initially be deterministic. Possible inputs include:

- exact match versus relaxed constraint;
- earliest acceptable time;
- preferred working windows;
- meeting clustering versus fragmentation;
- minimum gaps and buffers;
- host-specific template preferences;
- guest timezone convenience;
- existing contact status;
- approval requirement.

The model may explain the ranking, but should not silently alter it. Each result should carry machine-readable reason codes so explanations remain grounded.

## Security and safety requirements

- Treat all guest text as untrusted input.
- Do not expose calendar event names, participants, descriptions, or raw busy intervals.
- Scope public tools to a tenant, host, and active meeting template.
- Rate limit proposal and refinement calls.
- Validate all normalized constraints server-side.
- Require a final availability check before booking.
- Use idempotency keys for confirmation.
- Expire proposals and holds quickly.
- Avoid placing secure booking tokens in model-visible logs where possible.
- Preserve tenant isolation in every query.
- Record proposal, hold, confirmation, and agent actions in the audit trail.
- Keep a non-agent fallback when model parsing is unavailable or uncertain.

## MVP scope

### Included

1. One host and one guest.
2. Existing active meeting templates and scheduling rules.
3. Free-text date/time constraints.
4. Structured interpretation with explicit timezone.
5. Approximately three ranked proposals.
6. One or more refinement turns.
7. Guest name, email, agenda, and existing template questions.
8. Final server-side slot validation.
9. Immediate confirmation or the existing approval workflow.
10. **Show all times** calendar fallback.
11. Conversational rescheduling for token-authenticated bookings.

### Deferred

- autonomous email negotiation;
- unrestricted calendar modification;
- large multi-party scheduling;
- cross-organization identity federation;
- learning opaque ranking policies from user behavior;
- replacing the existing host dashboard;
- fully autonomous host-side decisions.

## Delivery plan

### Phase 0: API extraction

- Separate public slot calculation from HTMX rendering.
- Add JSON representations for public host, meeting type, and available slots.
- Add idempotent JSON booking creation.
- Keep existing HTML routes using the same services.

### Phase 1: Deterministic proposal engine

- Introduce structured scheduling constraints.
- Implement ranking and explicit relaxation rules.
- Add proposal expiry and final revalidation.
- Test timezone, DST, buffers, pooled hosts, and race conditions.

### Phase 2: Intent-first public UI

- Add the natural-language entry surface.
- Parse requests into structured constraints.
- Show proposal cards and grounded explanations.
- Support refinement and calendar fallback.
- Instrument conversion, retries, fallbacks, and booking time.

### Phase 3: Authenticated agent tools

- Expose narrowly scoped host tools through API and/or MCP.
- Support approval, alternative suggestions, and policy management.
- Add explicit confirmation boundaries for consequential actions.

### Phase 4: Multi-party negotiation

- Model participant constraints and proposal responses.
- Coordinate several humans or authorized agents.
- Add asynchronous negotiation channels.

## MVP success measures

Compare the intent-first experience with the current booking grid:

- completion rate;
- median time from page open to booking/request;
- number of interactions before confirmation;
- abandonment after seeing availability;
- percentage using **Show all times** fallback;
- interpretation correction rate;
- proposal expiry and slot-race rate;
- rescheduling completion rate;
- guest-reported confidence that the selected time matches their request.

A successful MVP should be at least as reliable as the current flow and materially better for approximate or constrained requests.

## Open questions

1. Should the first version use a hosted model, a configurable provider, or both?
2. Should public conversations be ephemeral or resumable through an emailed link?
3. How long should proposals and slot holds remain valid?
4. Which host preferences belong in templates versus a separate scheduling-policy model?
5. Should meeting-type inference be automatic, suggested, or always confirmed?
6. What information may the guest see when a requested interval is unavailable?
7. Should the initial agent integration be an internal tool API, an MCP server, or both?
8. Which operations require host confirmation in the authenticated agent experience?

## Recommended decision

Proceed with a narrow v2 spike that retains the current Go backend, extracts a structured public proposal API, and prototypes the intent-first guest journey for one host and one guest.

Do not begin by rewriting the dashboard or embedding a general-purpose chatbot. Validate the central proposition first:

> Can MeetWhen turn an approximate scheduling request into a small set of trustworthy proposals faster and more naturally than a calendar grid?
