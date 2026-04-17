import SwiftUI

struct BookingRowView: View {
    let booking: APIBooking
    let isNext: Bool
    let onJoin: () -> Void

    private var timeFormatter: DateFormatter {
        let f = DateFormatter()
        f.dateFormat = "h:mma"
        f.amSymbol = "a"
        f.pmSymbol = "p"
        return f
    }

    var body: some View {
        HStack(alignment: .top, spacing: 8) {
            // Time + indicator
            VStack(alignment: .trailing, spacing: 2) {
                if isNext {
                    Circle()
                        .fill(.green)
                        .frame(width: 6, height: 6)
                }
                Text(timeFormatter.string(from: booking.startTime))
                    .font(.system(.caption, design: .monospaced))
                    .foregroundStyle(isNext ? .primary : .secondary)
            }
            .frame(width: 55, alignment: .trailing)

            // Details
            VStack(alignment: .leading, spacing: 2) {
                Text(booking.inviteeName)
                    .font(.body)
                    .lineLimit(1)
                Text("\(booking.duration)min \(booking.templateName)")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }

            Spacer()

            // Join button
            if booking.hasConferenceLink {
                Button(action: onJoin) {
                    Label(booking.conferenceLabel, systemImage: "video.fill")
                        .font(.caption)
                }
                .buttonStyle(.borderedProminent)
                .controlSize(.small)
            }
        }
        .padding(.vertical, 4)
    }
}

struct PendingBookingRowView: View {
    let booking: APIBooking
    let onApprove: () -> Void
    let onReject: (String) -> Void

    @State private var showingRejectInput = false
    @State private var rejectReason = ""

    private var dateTimeFormatter: DateFormatter {
        let f = DateFormatter()
        f.dateFormat = "MMM d h:mma"
        f.amSymbol = "a"
        f.pmSymbol = "p"
        return f
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(booking.inviteeName)
                .font(.body)
                .lineLimit(1)
            Text("\(booking.duration)min \(booking.templateName) \u{00B7} \(dateTimeFormatter.string(from: booking.startTime))")
                .font(.caption)
                .foregroundStyle(.secondary)
                .lineLimit(1)

            if showingRejectInput {
                rejectInputView
            } else {
                actionButtons
            }
        }
        .padding(.vertical, 4)
    }

    private var actionButtons: some View {
        HStack(spacing: 8) {
            Spacer()
            Button(action: onApprove) {
                Label("Approve", systemImage: "checkmark")
                    .font(.caption)
            }
            .buttonStyle(.borderedProminent)
            .controlSize(.small)
            .tint(.green)

            Button(action: { showingRejectInput = true }) {
                Image(systemName: "xmark")
                    .font(.caption)
            }
            .buttonStyle(.bordered)
            .controlSize(.small)
        }
    }

    private var rejectInputView: some View {
        VStack(alignment: .leading, spacing: 6) {
            TextField("Reason (optional)", text: $rejectReason)
                .textFieldStyle(.roundedBorder)
                .font(.caption)

            HStack(spacing: 8) {
                Spacer()
                Button("Cancel") {
                    showingRejectInput = false
                    rejectReason = ""
                }
                .buttonStyle(.plain)
                .font(.caption)
                .foregroundStyle(.secondary)

                Button(action: {
                    onReject(rejectReason)
                    showingRejectInput = false
                    rejectReason = ""
                }) {
                    Label("Reject", systemImage: "xmark")
                        .font(.caption)
                }
                .buttonStyle(.borderedProminent)
                .controlSize(.small)
                .tint(.red)
            }
        }
    }
}
