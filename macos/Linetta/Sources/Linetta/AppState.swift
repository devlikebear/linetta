import Foundation
import LinettaCore

@MainActor
final class AppState: ObservableObject {
    @Published private(set) var works: [Work] = []
    @Published var selectedWork: Work?
    @Published var isLoading = false
    @Published var errorMessage: String?

    private let client: APIClient

    init(client: APIClient = APIClient()) {
        self.client = client
    }

    func refreshWorks() async {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }
        do {
            works = try await client.listWorks()
            if let selectedWork, !works.contains(where: { $0.id == selectedWork.id }) {
                self.selectedWork = nil
            }
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    func createWork(title: String, genre: String, premise: String) async {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }
        do {
            let work = try await client.createWork(CreateWorkRequest(title: title, genre: genre, premise: premise))
            works.insert(work, at: 0)
            selectedWork = work
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
