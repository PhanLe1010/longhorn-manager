package server

import (
	"net/http"
	"reflect"

	"github.com/rancher/wrangler/pkg/webhook"
	"github.com/sirupsen/logrus"

	"github.com/longhorn/longhorn-manager/webhook/admission"
)

func addHandler(router *webhook.Router, admissionType string, admitter admission.Admitter) {
	rsc := admitter.Resource()
	kind := reflect.Indirect(reflect.ValueOf(rsc.ObjectType)).Type().Name()
	router.Kind(kind).Group(rsc.APIGroup).Type(rsc.ObjectType).Handle(admission.NewHandler(admitter, admissionType))
	logrus.Infof("Add %s handler for %s.%s (%s)", admissionType, rsc.Name, rsc.APIGroup, kind)
}

type healthzHandler struct {
	Name string
}

func newhealthzHandler(name string) *healthzHandler {
	return &healthzHandler{
		Name: name,
	}
}

func (h *healthzHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	logrus.Infof("====================> type: %v. Someone check the health of me.", h.Name)
	w.WriteHeader(http.StatusOK)
	logrus.Infof("====================> type: %v. I sent the response back.", h.Name)
	return
}
