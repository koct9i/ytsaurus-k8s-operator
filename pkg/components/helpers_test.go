package components

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	"go.ytsaurus.tech/yt/go/ypath"
	"go.ytsaurus.tech/yt/go/yt"
	"go.ytsaurus.tech/yt/go/yterrors"

	mock_yt "github.com/ytsaurus/ytsaurus-k8s-operator/pkg/mock"
)

var _ = Describe("YTsaurus helper functions", func() {
	Describe("CreateUser", func() {
		var (
			mockCtrl     *gomock.Controller
			mockYtClient *mock_yt.MockClient
		)

		BeforeEach(func() {
			mockCtrl = gomock.NewController(GinkgoT())
			mockYtClient = mock_yt.NewMockClient(mockCtrl)
		})

		It("creates a user with token and superuser membership", func(ctx context.Context) {
			userName := "query_tracker"
			token := "test-token"
			tokenHash := sha256String(token)
			tokenPath := ypath.Path(fmt.Sprintf("//sys/cypress_tokens/%s", tokenHash))

			gomock.InOrder(
				mockYtClient.EXPECT().
					CreateObject(
						gomock.Any(),
						gomock.Eq(yt.NodeUser),
						gomock.Eq(&yt.CreateObjectOptions{
							IgnoreExisting: true,
							Attributes: map[string]interface{}{
								"name": userName,
							},
						}),
					).
					Return(yt.NodeID{}, nil),
				mockYtClient.EXPECT().
					CreateNode(
						gomock.Any(),
						gomock.Eq(tokenPath),
						gomock.Eq(yt.NodeMap),
						gomock.Eq(&yt.CreateNodeOptions{IgnoreExisting: true}),
					).
					Return(yt.NodeID{}, nil),
				mockYtClient.EXPECT().
					SetNode(
						gomock.Any(),
						gomock.Eq(tokenPath.Attr("user")),
						gomock.Eq(userName),
						gomock.Nil(),
					).
					Return(nil),
				mockYtClient.EXPECT().
					AddMember(gomock.Any(), gomock.Eq("superusers"), gomock.Eq(userName), gomock.Nil()).
					Return(nil),
			)

			Expect(CreateUser(ctx, mockYtClient, userName, token, true)).To(Succeed())
		})

		It("ignores an already-present superuser membership", func(ctx context.Context) {
			userName := "timbertruck"

			mockYtClient.EXPECT().
				CreateObject(gomock.Any(), gomock.Eq(yt.NodeUser), gomock.Any()).
				Return(yt.NodeID{}, nil)
			mockYtClient.EXPECT().
				AddMember(gomock.Any(), gomock.Eq("superusers"), gomock.Eq(userName), gomock.Nil()).
				Return(&yterrors.Error{Code: yterrors.CodeAlreadyPresentInGroup})

			Expect(CreateUser(ctx, mockYtClient, userName, "", true)).To(Succeed())
		})
	})
})
