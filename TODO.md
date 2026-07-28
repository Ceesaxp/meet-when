# TODO — Schedule Event fixes

## 2. Edit form time-zone shift (DATA CORRUPTION) — DONE
- [x] Add `dateInputInTZ` / `timeInputInTZ` template helpers (internal/handlers/helpers.go)
- [x] Register them in FuncMap (internal/handlers/handlers.go)
- [x] Edit form pre-fills date/time converted into event's timezone (dashboard_event_form.html)
- [x] Regression test: non-UTC event pre-fills local wall-clock

## 1. Timezone dropdown on Schedule Event form — DONE
- [x] Replace free-text timezone input with shared TimezonePicker
- [x] Wire onSelect -> conflict-check trigger
- [ ] (Left native time inputs / 24h alone per user — browser-locale issue, not server)

## 3. Host not notified + calendar time not updated — DONE
- [x] Add SendHostedEventUpdatedToHost to email interface + impl
- [x] Call it in HostedEventService.Update when attendees are notified
- [x] Include start/end (+summary) in updateGoogleEvent PATCH so host calendar time updates
- [x] Test: host gets exactly one update email

## Follow-up (needs Andrei's decision)
- [ ] Migration for already-shifted existing events — manual review recommended (see notes)
- [x] Build + full test suite green
