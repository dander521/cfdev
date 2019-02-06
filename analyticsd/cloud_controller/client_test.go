package cloud_controller_test

import (
	"code.cloudfoundry.org/cfdev/analyticsd/cloud_controller"
	"code.cloudfoundry.org/cfdev/analyticsd/cloud_controller/mocks"
	"encoding/json"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
	"gopkg.in/segmentio/analytics-go.v3"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"time"
)

var _ = Describe("Cloud Controller Client", func() {
	var (
		client         *cloud_controller.Client
		server         *ghttp.Server
		mockController *gomock.Controller
		mockAnalytics  *mocks.MockClient
	)

	BeforeEach(func() {
		server = ghttp.NewServer()
		mockController = gomock.NewController(GinkgoT())
		mockAnalytics = mocks.NewMockClient(mockController)

		client = cloud_controller.New(
			server.URL(),
			log.New(ioutil.Discard, "", log.LstdFlags),
			&http.Client{},
			mockAnalytics,
			"some-user-id",
			"some-version",
			"false",
		)
	})

	AfterEach(func() {
		server.Close()
		mockController.Finish()
	})

	Describe("FetchLatestTime", func() {
		Context("when events are returned", func() {
			It("returns the latest timestamp of the events", func() {
				server.AppendHandlers(ghttp.CombineHandlers(
					ghttp.VerifyRequest(http.MethodGet, "/v2/events"),
					ghttp.RespondWith(http.StatusOK, `
					{
						"resources" : [
							{
								"entity": {
									 "timestamp": "2016-06-06T06:06:06Z"
								}
							},
							{
								"entity": {
									 "timestamp": "some-non-considered-timestamp"
								}
							}
						]
					}
					`)),
				)

				t := client.FetchLatestTime()
				Expect(t).To(BeTemporally("==", time.Date(2016, 6, 6, 6, 6, 6, 0, time.UTC)))
			})
		})

		Context("when events are not returned", func() {
			It("returns a current timestamp", func() {
				server.AppendHandlers(ghttp.CombineHandlers(
					ghttp.VerifyRequest(http.MethodGet, "/v2/events"),
					ghttp.RespondWith(http.StatusOK, `
					{
						"resources" : []
					}
					`)),
				)

				t := client.FetchLatestTime()
				Expect(t).To(BeTemporally("~", time.Now().UTC(), time.Minute))
			})
		})

		Context("when there is an error retrieving the timestamp", func() {
			It("returns a current timestamp", func() {
				mockAnalytics.EXPECT().Enqueue(gomock.Any())

				server.AppendHandlers(ghttp.CombineHandlers(
					ghttp.VerifyRequest(http.MethodGet, "/v2/events"),
					ghttp.RespondWith(http.StatusInternalServerError, `some-error-message`)),
				)

				t := client.FetchLatestTime()
				Expect(t).To(BeTemporally("~", time.Now().UTC(), time.Minute))
			})
		})
	})

	Describe("FetchEvents", func() {
		Context("when there is a page of results", func() {
			It("returns events from the page", func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest(http.MethodGet, "/v2/events"),
						ghttp.RespondWith(http.StatusOK, `
						{
							"next_url": null,
							"resources": [{
								"entity": {
									"type": "some-event-type",
									"timestamp": "2016-06-06T06:06:06Z",
									"metadata": "some-metadata"
								}
							}]
						}
					`)),
				)

				events, err := client.FetchEvents(time.Time{})
				Expect(err).NotTo(HaveOccurred())

				Expect(events).To(Equal([]cloud_controller.Event{
					{
						Type:      "some-event-type",
						Timestamp: time.Date(2016, 6, 6, 6, 6, 6, 0, time.UTC),
						Metadata:  json.RawMessage(`"some-metadata"`),
					},
				}))
			})
		})

		Context("when there are multiple pages of results", func() {
			It("returns events from all pages", func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest(http.MethodGet, "/v2/events"),
						func(w http.ResponseWriter, req *http.Request) {
							values := req.URL.Query()
							Expect(values["q"][0]).To(ContainSubstring("type IN"))
							Expect(values["q"][1]).To(ContainSubstring("timestamp"))
						},
						ghttp.RespondWith(http.StatusOK, `
						{
							"next_url": "/v546/events?page=2&some_key=some_value",
							"resources": [{
								"entity": {
									"type": "some-event-type",
									"timestamp": "2016-06-06T06:06:06Z",
									"metadata": "some-metadata"
								}
							}]
						}
					`)),
					ghttp.CombineHandlers(
						ghttp.VerifyRequest(http.MethodGet, "/v2/events"),
						func(w http.ResponseWriter, req *http.Request) {
							values := req.URL.Query()
							Expect(values.Get("page")).To(Equal("2"))
							Expect(values.Get("some_key")).To(Equal("some_value"))
						},
						ghttp.RespondWith(http.StatusOK, `
						{
							"next_url": null,
							"resources": [{
								"entity": {
									"type": "some-other-event-type",
									"timestamp": "2016-07-07T07:07:07Z",
									"metadata": "some-other-metadata"
								}
							}]
						}
					`)),
				)

				events, err := client.FetchEvents(time.Time{})
				Expect(err).NotTo(HaveOccurred())

				Expect(events).To(Equal([]cloud_controller.Event{
					{
						Type:      "some-event-type",
						Timestamp: time.Date(2016, 6, 6, 6, 6, 6, 0, time.UTC),
						Metadata:  json.RawMessage(`"some-metadata"`),
					},
					{
						Type:      "some-other-event-type",
						Timestamp: time.Date(2016, 7, 7, 7, 7, 7, 0, time.UTC),
						Metadata:  json.RawMessage(`"some-other-metadata"`),
					},
				}))
			})
		})
	})

	Describe("Fetch", func() {
		Context("when supplied params and destination", func() {
			It("makes a request and unmarshals the response", func() {
				server.AppendHandlers(ghttp.CombineHandlers(
					ghttp.VerifyRequest(http.MethodGet, "/v2/some-endpoint", "q=banana&page=4"),
					ghttp.RespondWith(http.StatusOK, `{"message": "some-success-message"}`),
				))

				var result struct {
					Message string
				}

				params := url.Values{}
				params.Add("q", "banana")
				params.Add("page", "4")

				err := client.Fetch("/v2/some-endpoint", params, &result)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Message).To(Equal("some-success-message"))
			})
		})

		Context("when cloud controller returns error status", func() {
			It("sends the error to segment.io", func() {
				server.AppendHandlers(ghttp.CombineHandlers(
					ghttp.VerifyRequest(http.MethodGet, "/v2/some-endpoint"),
					ghttp.RespondWith(http.StatusForbidden, `some-failure-message`),
				))

				mockAnalytics.EXPECT().Enqueue(gomock.Any()).Do(func(message analytics.Track) {
					Expect(message.Event).To(Equal("analytics error"))
					Expect(message.UserId).To(Equal("some-user-id"))
					Expect(message.Properties).To(HaveKeyWithValue(
						"message", "failed to contact cc api: [403 Forbidden] some-failure-message",
					))
				})

				err := client.Fetch("/v2/some-endpoint", nil, nil)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
