package resources

import (
	"context"

	"github.com/ytsaurus/yt-k8s-operator/pkg/apiproxy"
	"github.com/ytsaurus/yt-k8s-operator/pkg/consts"
	labeller2 "github.com/ytsaurus/yt-k8s-operator/pkg/labeller"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RPCService struct {
	name     string
	labeller *labeller2.Labeller
	apiProxy apiproxy.APIProxy

	oldObject corev1.Service
	newObject corev1.Service
}

func NewRPCService(name string, labeller *labeller2.Labeller, apiProxy apiproxy.APIProxy) *RPCService {
	return &RPCService{
		name:     name,
		labeller: labeller,
		apiProxy: apiProxy,
	}
}

func (s *RPCService) Service() corev1.Service {
	return s.oldObject
}

func (s *RPCService) OldObject() client.Object {
	return &s.oldObject
}

func (s *RPCService) Name() string {
	return s.name
}

func (s *RPCService) Sync(ctx context.Context) error {
	return s.apiProxy.SyncObject(ctx, &s.oldObject, &s.newObject)
}

func (s *RPCService) Build() *corev1.Service {
	s.newObject.ObjectMeta = s.labeller.GetObjectMeta(s.name)
	s.newObject.Spec = corev1.ServiceSpec{
		Selector: s.labeller.GetSelectorLabelMap(nil),
		Ports: []corev1.ServicePort{
			{
				Name:       "rpc",
				Port:       consts.RPCProxyRPCPort,
				TargetPort: intstr.IntOrString{IntVal: consts.RPCProxyRPCPort},
			},
		},
	}

	return &s.newObject
}

func (s *RPCService) Fetch(ctx context.Context) error {
	return s.apiProxy.FetchObject(ctx, s.name, &s.oldObject)
}
