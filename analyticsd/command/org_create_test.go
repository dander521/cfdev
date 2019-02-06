package command_test

import (
	"code.cloudfoundry.org/cfdev/analyticsd/command"
	"code.cloudfoundry.org/cfdev/analyticsd/command/mocks"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gopkg.in/segmentio/analytics-go.v3"
	"io/ioutil"
	"log"
	"runtime"
	"time"
)

var _ = Describe("OrgCreate", func() {
	var (
		cmd            *command.OrgCreate
		mockController *gomock.Controller
		mockAnalytics  *mocks.MockClient
		mockCCClient   *mocks.MockCloudControllerClient
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		mockAnalytics = mocks.NewMockClient(mockController)
		mockCCClient = mocks.NewMockCloudControllerClient(mockController)

		cmd = &command.OrgCreate{
			Logger:          log.New(ioutil.Discard, "", log.LstdFlags),
			CCClient:        mockCCClient,
			AnalyticsClient: mockAnalytics,
			TimeStamp:       time.Date(2018, 7, 7, 7, 7, 7, 0, time.UTC),
			UUID:            "some-user-uuid",
			Version:         "some-version",
			OSVersion:       "some-os-version",
			IsBehindProxy:   "false",
		}
	})

	AfterEach(func() {
		mockController.Finish()
	})

	Context("when org is created", func() {
		It("sends the org information to segment.io", func() {
			mockAnalytics.EXPECT().Enqueue(analytics.Track{
				UserId:    "some-user-uuid",
				Event:     "org created",
				Timestamp: time.Date(2018, 7, 7, 7, 7, 7, 0, time.UTC),
				Properties: map[string]interface{}{
					"os":             runtime.GOOS,
					"plugin_version": "some-version",
					"os_version":     "some-os-version",
					"proxy":          "false",
				},
			})

			body := []byte("")

			Expect(cmd.HandleResponse(body)).NotTo(HaveOccurred())
		})
	})
})
