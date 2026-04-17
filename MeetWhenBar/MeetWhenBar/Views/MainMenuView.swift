import SwiftUI

struct MainMenuView: View {
    @Bindable var viewModel: AppViewModel

    var body: some View {
        if viewModel.showingSettings {
            SettingsView(viewModel: viewModel)
        } else {
            mainContent
        }
    }

    private var mainContent: some View {
        VStack(spacing: 0) {
            // Header
            header
            Divider()

            // Error banner
            if let error = viewModel.errorMessage {
                errorBanner(error)
            }

            ScrollView {
                VStack(alignment: .leading, spacing: 0) {
                    // Pending section
                    if !viewModel.pendingBookings.isEmpty {
                        pendingSection
                        Divider()
                    }

                    // Today section
                    todaySection
                }
            }
            .frame(maxHeight: 400)

            Divider()
            footer
        }
        .frame(width: 320)
    }

    // MARK: - Header

    private var header: some View {
        HStack {
            VStack(alignment: .leading, spacing: 1) {
                Text("Meet When")
                    .font(.headline)
                if let tenant = viewModel.tenant {
                    Text(tenant.name)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            Spacer()
            if viewModel.isLoading {
                ProgressView()
                    .controlSize(.small)
            }
            Button(action: { viewModel.showingSettings.toggle() }) {
                Image(systemName: "gearshape")
                    .foregroundStyle(.secondary)
            }
            .buttonStyle(.plain)
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
    }

    // MARK: - Pending approvals

    private var pendingSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            Label("Pending Approval (\(viewModel.pendingBookings.count))", systemImage: "clock.badge.questionmark")
                .font(.caption)
                .fontWeight(.semibold)
                .foregroundStyle(.orange)
                .padding(.horizontal, 16)
                .padding(.top, 8)

            ForEach(viewModel.pendingBookings) { booking in
                PendingBookingRowView(
                    booking: booking,
                    onApprove: {
                        Task { await viewModel.approveBooking(booking.id) }
                    },
                    onReject: { reason in
                        Task { await viewModel.rejectBooking(booking.id, reason: reason) }
                    }
                )
                .padding(.horizontal, 16)
            }
            .padding(.bottom, 8)
        }
    }

    // MARK: - Today's bookings

    private var todaySection: some View {
        VStack(alignment: .leading, spacing: 4) {
            Label("Today (\(viewModel.todayBookings.count))", systemImage: "calendar")
                .font(.caption)
                .fontWeight(.semibold)
                .foregroundStyle(.secondary)
                .padding(.horizontal, 16)
                .padding(.top, 8)

            if viewModel.todayBookings.isEmpty {
                Text("No meetings today")
                    .font(.caption)
                    .foregroundStyle(.tertiary)
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 16)
            } else {
                ForEach(Array(viewModel.todayBookings.enumerated()), id: \.element.id) { index, booking in
                    BookingRowView(
                        booking: booking,
                        isNext: isNextBooking(booking),
                        onJoin: {
                            viewModel.openConferenceLink(booking.conferenceLink)
                        }
                    )
                    .padding(.horizontal, 16)

                    if index < viewModel.todayBookings.count - 1 {
                        Divider()
                            .padding(.leading, 72)
                    }
                }
            }
        }
        .padding(.bottom, 8)
    }

    // MARK: - Footer

    private var footer: some View {
        HStack {
            Button("Open Dashboard") {
                viewModel.openDashboard()
            }
            .buttonStyle(.plain)
            .font(.caption)
            .foregroundStyle(.blue)

            Spacer()

            Button("Sign Out") {
                Task { await viewModel.logout() }
            }
            .buttonStyle(.plain)
            .font(.caption)
            .foregroundStyle(.secondary)

            Divider()
                .frame(height: 12)

            Button("Quit") {
                NSApplication.shared.terminate(nil)
            }
            .buttonStyle(.plain)
            .font(.caption)
            .foregroundStyle(.secondary)
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 8)
    }

    // MARK: - Error banner

    private func errorBanner(_ message: String) -> some View {
        HStack(spacing: 6) {
            Image(systemName: "exclamationmark.triangle.fill")
                .foregroundStyle(.orange)
                .font(.caption)
            Text(message)
                .font(.caption)
                .lineLimit(2)
            Spacer()
            Button(action: { viewModel.errorMessage = nil }) {
                Image(systemName: "xmark")
                    .font(.caption2)
            }
            .buttonStyle(.plain)
            .foregroundStyle(.secondary)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 6)
        .background(.orange.opacity(0.1))
    }

    // MARK: - Helpers

    /// Returns true if this is the next upcoming (or currently active) booking.
    private func isNextBooking(_ booking: APIBooking) -> Bool {
        let now = Date()
        // Currently active
        if booking.startTime <= now && booking.endTime > now { return true }
        // First future booking
        if booking.startTime > now {
            return viewModel.todayBookings.first(where: { $0.startTime > now })?.id == booking.id
        }
        return false
    }
}
