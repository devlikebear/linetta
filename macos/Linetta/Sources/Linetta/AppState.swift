import Combine
import Foundation
import LinettaCore
import Observation

@MainActor
@Observable
final class AppState {
    private(set) var works: [Work] = []
    var selectedWork: Work?
    var isLoading = false
    var errorMessage: String?
    private(set) var client: APIClient

    private let engine: EngineController
    @ObservationIgnored private var cancellables: Set<AnyCancellable> = []

    init(engine: EngineController) {
        self.engine = engine
        self.client = APIClient(baseURL: engine.address ?? APIClient.defaultBaseURL)
        engine.$address
            .dropFirst()
            .receive(on: RunLoop.main)
            .sink { [weak self] address in
                guard let self else { return }
                self.client = APIClient(baseURL: address ?? APIClient.defaultBaseURL)
                if address != nil {
                    Task { @MainActor [weak self] in await self?.refreshWorks() }
                }
            }
            .store(in: &cancellables)
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
