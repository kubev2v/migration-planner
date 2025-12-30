package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/kubev2v/migration-planner/internal/client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("sizer client", func() {
	var (
		sizerClient *client.SizerClient
		ctx         context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("NewSizerClient", func() {
		It("creates client with default timeout when timeout is 0", func() {
			c := client.NewSizerClient("http://localhost:8080", 0)
			Expect(c).NotTo(BeNil())
		})

		It("creates client with custom timeout", func() {
			customTimeout := 30 * time.Second
			c := client.NewSizerClient("http://localhost:8080", customTimeout)
			Expect(c).NotTo(BeNil())
		})
	})

	Describe("CalculateSizing", func() {
		Context("successful requests", func() {
			It("successfully calculates sizing", func() {
				expectedResponse := &client.SizerResponse{
					Success: true,
					Data: client.SizerData{
						NodeCount:   5,
						TotalCPU:    40,
						TotalMemory: 80,
						ResourceConsumption: client.ResourceConsumption{
							CPU:    100.0,
							Memory: 200.0,
						},
					},
				}

				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.Method).To(Equal(http.MethodPost))
					Expect(r.URL.Path).To(Equal("/api/v1/size/custom"))
					Expect(r.Header.Get("Content-Type")).To(Equal("application/json"))

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(expectedResponse)
				}))
				defer server.Close()

				sizerClient = client.NewSizerClient(server.URL, 5*time.Second)

				request := &client.SizerRequest{
					Platform: "BAREMETAL",
					MachineSets: []client.MachineSet{
						{
							Name:   "worker",
							CPU:    8,
							Memory: 16,
						},
					},
					Workloads: []client.Workload{},
					Detailed:  true,
				}

				response, err := sizerClient.CalculateSizing(ctx, request)

				Expect(err).To(BeNil())
				Expect(response).NotTo(BeNil())
				Expect(response.Success).To(BeTrue())
				Expect(response.Data.NodeCount).To(Equal(5))
			})

			It("correctly marshals request body", func() {
				var receivedRequest client.SizerRequest

				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_ = json.NewDecoder(r.Body).Decode(&receivedRequest)

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(&client.SizerResponse{
						Success: true,
						Data:    client.SizerData{NodeCount: 1},
					})
				}))
				defer server.Close()

				sizerClient = client.NewSizerClient(server.URL, 5*time.Second)

				request := &client.SizerRequest{
					Platform: "BAREMETAL",
					MachineSets: []client.MachineSet{
						{Name: "worker", CPU: 8, Memory: 16},
					},
					Workloads: []client.Workload{},
					Detailed:  true,
				}

				_, err := sizerClient.CalculateSizing(ctx, request)

				Expect(err).To(BeNil())
				Expect(receivedRequest.Platform).To(Equal("BAREMETAL"))
				Expect(receivedRequest.Detailed).To(BeTrue())
			})
		})

		Context("error handling", func() {
			It("returns error when HTTP request creation fails", func() {
				// Invalid URL should cause request creation to fail
				sizerClient = client.NewSizerClient("http://[invalid-url", 5*time.Second)

				request := &client.SizerRequest{
					Platform:    "BAREMETAL",
					MachineSets: []client.MachineSet{},
					Workloads:   []client.Workload{},
				}

				_, err := sizerClient.CalculateSizing(ctx, request)

				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("failed to create request"))
			})

			It("returns error when HTTP client Do() fails", func() {
				// Use unreachable IP address to trigger connection failure
				sizerClient = client.NewSizerClient("http://192.0.2.0:8080", 1*time.Second)

				request := &client.SizerRequest{
					Platform:    "BAREMETAL",
					MachineSets: []client.MachineSet{},
					Workloads:   []client.Workload{},
				}

				_, err := sizerClient.CalculateSizing(ctx, request)

				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("failed to call sizer service"))
			})

			It("returns error when response status is not 200", func() {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte("Internal Server Error"))
				}))
				defer server.Close()

				sizerClient = client.NewSizerClient(server.URL, 5*time.Second)

				request := &client.SizerRequest{
					Platform:    "BAREMETAL",
					MachineSets: []client.MachineSet{},
					Workloads:   []client.Workload{},
				}

				_, err := sizerClient.CalculateSizing(ctx, request)

				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("status 500"))
			})

			It("returns error when response body read fails", func() {
				// Use a server with invalid Content-Length to simulate read failure
				// Setting Content-Length larger than actual body causes ReadAll to fail
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("Content-Length", "1000") // Claim 1000 bytes
					w.WriteHeader(http.StatusOK)
					// But only write a few bytes, causing ReadAll to fail waiting for remaining data
					_, _ = w.Write([]byte("{"))
				}))
				defer server.Close()

				sizerClient = client.NewSizerClient(server.URL, 5*time.Second)

				request := &client.SizerRequest{
					Platform:    "BAREMETAL",
					MachineSets: []client.MachineSet{},
					Workloads:   []client.Workload{},
				}

				_, err := sizerClient.CalculateSizing(ctx, request)

				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("failed to read response body"))
			})

			It("returns error when JSON unmarshal fails", func() {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("{invalid json}"))
				}))
				defer server.Close()

				sizerClient = client.NewSizerClient(server.URL, 5*time.Second)

				request := &client.SizerRequest{
					Platform:    "BAREMETAL",
					MachineSets: []client.MachineSet{},
					Workloads:   []client.Workload{},
				}

				_, err := sizerClient.CalculateSizing(ctx, request)

				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("failed to decode response"))
			})

			It("returns error when Success is false", func() {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(&client.SizerResponse{
						Success: false,
						Error:   "calculation failed",
					})
				}))
				defer server.Close()

				sizerClient = client.NewSizerClient(server.URL, 5*time.Second)

				request := &client.SizerRequest{
					Platform:    "BAREMETAL",
					MachineSets: []client.MachineSet{},
					Workloads:   []client.Workload{},
				}

				_, err := sizerClient.CalculateSizing(ctx, request)

				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("calculation failed"))
			})
		})

		Context("context handling", func() {
			It("respects context cancellation", func() {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(2 * time.Second) // Delay response
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(&client.SizerResponse{Success: true, Data: client.SizerData{}})
				}))
				defer server.Close()

				sizerClient = client.NewSizerClient(server.URL, 5*time.Second)

				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately

				request := &client.SizerRequest{
					Platform:    "BAREMETAL",
					MachineSets: []client.MachineSet{},
					Workloads:   []client.Workload{},
				}

				_, err := sizerClient.CalculateSizing(ctx, request)

				Expect(err).NotTo(BeNil())
			})

			It("respects context timeout", func() {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(2 * time.Second) // Delay response
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(&client.SizerResponse{Success: true, Data: client.SizerData{}})
				}))
				defer server.Close()

				sizerClient = client.NewSizerClient(server.URL, 5*time.Second)

				ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				defer cancel()

				request := &client.SizerRequest{
					Platform:    "BAREMETAL",
					MachineSets: []client.MachineSet{},
					Workloads:   []client.Workload{},
				}

				_, err := sizerClient.CalculateSizing(ctx, request)

				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("HealthCheck", func() {
		Context("successful requests", func() {
			It("successfully returns nil when health check succeeds", func() {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.Method).To(Equal(http.MethodGet))
					Expect(r.URL.Path).To(Equal("/health"))
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()

				sizerClient = client.NewSizerClient(server.URL, 5*time.Second)

				err := sizerClient.HealthCheck(ctx)

				Expect(err).To(BeNil())
			})
		})

		Context("error handling", func() {
			It("returns error when HTTP request creation fails", func() {
				sizerClient = client.NewSizerClient("http://[invalid-url", 5*time.Second)

				err := sizerClient.HealthCheck(ctx)

				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("failed to create request"))
			})

			It("returns error when HTTP client Do() fails", func() {
				// Use unreachable IP address to trigger connection failure
				sizerClient = client.NewSizerClient("http://192.0.2.0:8080", 1*time.Second)

				err := sizerClient.HealthCheck(ctx)

				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("failed to call sizer service"))
			})

			It("returns error when status code is not 200", func() {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusServiceUnavailable)
				}))
				defer server.Close()

				sizerClient = client.NewSizerClient(server.URL, 5*time.Second)

				err := sizerClient.HealthCheck(ctx)

				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("status 503"))
			})
		})

		Context("context handling", func() {
			It("respects context cancellation", func() {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(2 * time.Second)
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()

				sizerClient = client.NewSizerClient(server.URL, 5*time.Second)

				ctx, cancel := context.WithCancel(context.Background())
				cancel()

				err := sizerClient.HealthCheck(ctx)

				Expect(err).NotTo(BeNil())
			})

			It("respects context timeout", func() {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(2 * time.Second)
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()

				sizerClient = client.NewSizerClient(server.URL, 5*time.Second)

				ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				defer cancel()

				err := sizerClient.HealthCheck(ctx)

				Expect(err).NotTo(BeNil())
			})
		})
	})
})
