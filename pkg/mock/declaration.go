package mock_yt

//go:generate go tool mockgen -destination=mock_ytsaurus_client.go -package=mock_yt go.ytsaurus.tech/yt/go/yt Client
