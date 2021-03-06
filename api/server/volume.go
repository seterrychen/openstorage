package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/libopenstorage/openstorage/api"
	"github.com/libopenstorage/openstorage/config"
	"github.com/libopenstorage/openstorage/volume/drivers"
)

type volApi struct {
	restBase
}

func responseStatus(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func newVolumeAPI(name string) restServer {
	return &volApi{restBase{version: config.Version, name: name}}
}

func (vd *volApi) String() string {
	return vd.name
}

func (vd *volApi) parseVolumeID(r *http.Request) (string, error) {
	vars := mux.Vars(r)
	if id, ok := vars["id"]; ok {
		return string(id), nil
	}
	return "", fmt.Errorf("could not parse snap ID")
}

func (vd *volApi) create(w http.ResponseWriter, r *http.Request) {
	var dcRes api.VolumeCreateResponse
	var dcReq api.VolumeCreateRequest
	method := "create"

	if err := json.NewDecoder(r.Body).Decode(&dcReq); err != nil {
		vd.sendError(vd.name, method, w, err.Error(), http.StatusBadRequest)
		return
	}

	d, err := volumedrivers.Get(vd.name)
	if err != nil {
		notFound(w, r)
		return
	}
	id, err := d.Create(dcReq.Locator, dcReq.Source, dcReq.Spec)
	dcRes.VolumeResponse = &api.VolumeResponse{Error: responseStatus(err)}
	dcRes.Id = id

	vd.logRequest(method, id).Infoln("")

	json.NewEncoder(w).Encode(&dcRes)
}

func (vd *volApi) volumeSet(w http.ResponseWriter, r *http.Request) {
	var (
		volumeID string
		err      error
		req      api.VolumeSetRequest
		resp     api.VolumeSetResponse
	)
	method := "volumeSet"

	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		vd.sendError(vd.name, method, w, err.Error(), http.StatusBadRequest)
		return
	}

	if volumeID, err = vd.parseVolumeID(r); err != nil {
		vd.sendError(vd.name, method, w, err.Error(), http.StatusBadRequest)
		return
	}

	vd.logRequest(method, string(volumeID)).Infoln("")

	d, err := volumedrivers.Get(vd.name)
	if err != nil {
		notFound(w, r)
		return
	}

	if req.Locator != nil || req.Spec != nil {
		err = d.Set(volumeID, req.Locator, req.Spec)
	}

	for err == nil && req.Action != nil {
		if req.Action.Attach != api.VolumeActionParam_VOLUME_ACTION_PARAM_NONE {
			if req.Action.Attach == api.VolumeActionParam_VOLUME_ACTION_PARAM_ON {
				_, err = d.Attach(volumeID)
			} else {
				err = d.Detach(volumeID)
			}
			if err != nil {
				break
			}
		}
		if req.Action.Mount != api.VolumeActionParam_VOLUME_ACTION_PARAM_NONE {
			if req.Action.Mount == api.VolumeActionParam_VOLUME_ACTION_PARAM_ON {
				if req.Action.MountPath == "" {
					err = fmt.Errorf("Invalid mount path")
					break
				}
				err = d.Mount(volumeID, req.Action.MountPath)
			} else {
				err = d.Unmount(volumeID, req.Action.MountPath)
			}
			if err != nil {
				break
			}
		}
		break
	}

	if err != nil {
		resp.VolumeResponse = &api.VolumeResponse{
			Error: err.Error(),
		}
	} else {
		v, err := d.Inspect([]string{volumeID})
		if err != nil || v == nil || len(v) != 1 {
			if err == nil {
				err = fmt.Errorf("Failed to inspect volume")
			}
			resp.VolumeResponse = &api.VolumeResponse{
				Error: err.Error(),
			}
		} else {
			v0 := v[0]
			resp.Volume = v0
		}
	}
	json.NewEncoder(w).Encode(resp)
}

func (vd *volApi) inspect(w http.ResponseWriter, r *http.Request) {
	var err error
	var volumeID string

	method := "inspect"
	d, err := volumedrivers.Get(vd.name)
	if err != nil {
		notFound(w, r)
		return
	}

	if volumeID, err = vd.parseVolumeID(r); err != nil {
		e := fmt.Errorf("Failed to parse parse volumeID: %s", err.Error())
		vd.sendError(vd.name, method, w, e.Error(), http.StatusBadRequest)
		return
	}

	vd.logRequest(method, string(volumeID)).Infoln("")

	dk, err := d.Inspect([]string{volumeID})
	if err != nil {
		vd.sendError(vd.name, method, w, err.Error(), http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(dk)
}

func (vd *volApi) delete(w http.ResponseWriter, r *http.Request) {
	var volumeID string
	var err error

	method := "delete"
	if volumeID, err = vd.parseVolumeID(r); err != nil {
		e := fmt.Errorf("Failed to parse parse volumeID: %s", err.Error())
		vd.sendError(vd.name, method, w, e.Error(), http.StatusBadRequest)
		return
	}

	vd.logRequest(method, volumeID).Infoln("")

	d, err := volumedrivers.Get(vd.name)
	if err != nil {
		notFound(w, r)
		return
	}

	volumeResponse := &api.VolumeResponse{}
	if err := d.Delete(volumeID); err != nil {
		volumeResponse.Error = err.Error()
	}
	json.NewEncoder(w).Encode(volumeResponse)
}

func (vd *volApi) enumerate(w http.ResponseWriter, r *http.Request) {
	var locator api.VolumeLocator
	var configLabels map[string]string
	var err error
	var vols []*api.Volume

	method := "enumerate"

	d, err := volumedrivers.Get(vd.name)
	if err != nil {
		notFound(w, r)
		return
	}
	params := r.URL.Query()
	v := params[string(api.OptName)]
	if v != nil {
		locator.Name = v[0]
	}
	v = params[string(api.OptLabel)]
	if v != nil {
		if err = json.Unmarshal([]byte(v[0]), &locator.VolumeLabels); err != nil {
			e := fmt.Errorf("Failed to parse parse VolumeLabels: %s", err.Error())
			vd.sendError(vd.name, method, w, e.Error(), http.StatusBadRequest)
		}
	}
	v = params[string(api.OptConfigLabel)]
	if v != nil {
		if err = json.Unmarshal([]byte(v[0]), &configLabels); err != nil {
			e := fmt.Errorf("Failed to parse parse configLabels: %s", err.Error())
			vd.sendError(vd.name, method, w, e.Error(), http.StatusBadRequest)
		}
	}
	v = params[string(api.OptVolumeID)]
	if v != nil {
		ids := make([]string, len(v))
		for i, s := range v {
			ids[i] = string(s)
		}
		vols, err = d.Inspect(ids)
		if err != nil {
			e := fmt.Errorf("Failed to inspect volumeID: %s", err.Error())
			vd.sendError(vd.name, method, w, e.Error(), http.StatusBadRequest)
			return
		}
	} else {
		vols, _ = d.Enumerate(&locator, configLabels)
	}
	json.NewEncoder(w).Encode(vols)
}

func (vd *volApi) snap(w http.ResponseWriter, r *http.Request) {
	var snapReq api.SnapCreateRequest
	var snapRes api.SnapCreateResponse
	method := "snap"

	if err := json.NewDecoder(r.Body).Decode(&snapReq); err != nil {
		vd.sendError(vd.name, method, w, err.Error(), http.StatusBadRequest)
		return
	}
	d, err := volumedrivers.Get(vd.name)
	if err != nil {
		notFound(w, r)
		return
	}

	vd.logRequest(method, string(snapReq.Id)).Infoln("")

	id, err := d.Snapshot(snapReq.Id, snapReq.Readonly, snapReq.Locator)
	snapRes.VolumeCreateResponse = &api.VolumeCreateResponse{
		Id: id,
		VolumeResponse: &api.VolumeResponse{
			Error: responseStatus(err),
		},
	}
	json.NewEncoder(w).Encode(&snapRes)
}

func (vd *volApi) snapEnumerate(w http.ResponseWriter, r *http.Request) {
	var err error
	var labels map[string]string
	var ids []string

	method := "snapEnumerate"
	d, err := volumedrivers.Get(vd.name)
	if err != nil {
		notFound(w, r)
		return
	}
	params := r.URL.Query()
	v := params[string(api.OptLabel)]
	if v != nil {
		if err = json.Unmarshal([]byte(v[0]), &labels); err != nil {
			e := fmt.Errorf("Failed to parse parse VolumeLabels: %s", err.Error())
			vd.sendError(vd.name, method, w, e.Error(), http.StatusBadRequest)
		}
	}

	v, ok := params[string(api.OptVolumeID)]
	if v != nil && ok {
		ids = make([]string, len(params))
		for i, s := range v {
			ids[i] = string(s)
		}
	}

	snaps, err := d.SnapEnumerate(ids, labels)
	if err != nil {
		e := fmt.Errorf("Failed to enumerate snaps: %s", err.Error())
		vd.sendError(vd.name, method, w, e.Error(), http.StatusBadRequest)
		return
	}

	json.NewEncoder(w).Encode(snaps)
}

func (vd *volApi) stats(w http.ResponseWriter, r *http.Request) {
	var volumeID string
	var err error

	method := "stats"
	if volumeID, err = vd.parseVolumeID(r); err != nil {
		e := fmt.Errorf("Failed to parse parse volumeID: %s", err.Error())
		vd.sendError(vd.name, method, w, e.Error(), http.StatusBadRequest)
		return
	}

	vd.logRequest(method, string(volumeID)).Infoln("")

	d, err := volumedrivers.Get(vd.name)
	if err != nil {
		notFound(w, r)
		return
	}

	stats, err := d.Stats(volumeID)
	if err != nil {
		e := fmt.Errorf("Failed to get stats: %s", err.Error())
		vd.sendError(vd.name, method, w, e.Error(), http.StatusBadRequest)
		return
	}
	json.NewEncoder(w).Encode(stats)
}

func (vd *volApi) alerts(w http.ResponseWriter, r *http.Request) {
	var volumeID string
	var err error

	method := "alerts"
	if volumeID, err = vd.parseVolumeID(r); err != nil {
		e := fmt.Errorf("Failed to parse parse volumeID: %s", err.Error())
		vd.sendError(vd.name, method, w, e.Error(), http.StatusBadRequest)
		return
	}

	d, err := volumedrivers.Get(vd.name)
	if err != nil {
		notFound(w, r)
		return
	}

	alerts, err := d.Alerts(volumeID)
	if err != nil {
		e := fmt.Errorf("Failed to get alerts: %s", err.Error())
		vd.sendError(vd.name, method, w, e.Error(), http.StatusBadRequest)
		return
	}
	json.NewEncoder(w).Encode(alerts)
}

func (vd *volApi) requests(w http.ResponseWriter, r *http.Request) {
	var volumeID string
	var err error

	method := "requests"
	if volumeID, err = vd.parseVolumeID(r); err != nil {
		e := fmt.Errorf("Failed to parse parse volumeID: %s", err.Error())
		vd.sendError(vd.name, method, w, e.Error(), http.StatusBadRequest)
		return
	}

	d, err := volumedrivers.Get(vd.name)
	if err != nil {
		notFound(w, r)
		return
	}

	requests, err := d.DumpRequests(volumeID)
	if err != nil {
		e := fmt.Errorf("Failed to get active requests: %s", err.Error())
		vd.sendError(vd.name, method, w, e.Error(), http.StatusBadRequest)
		return
	}
	json.NewEncoder(w).Encode(requests)
}

func volVersion(route string) string {
	return "/" + config.Version + "/" + route
}

func volPath(route string) string {
	return volVersion("volumes" + route)
}

func snapPath(route string) string {
	return volVersion("snapshot" + route)
}

func (vd *volApi) Routes() []*Route {
	return []*Route{
		&Route{verb: "POST", path: volPath(""), fn: vd.create},
		&Route{verb: "PUT", path: volPath("/{id}"), fn: vd.volumeSet},
		&Route{verb: "GET", path: volPath(""), fn: vd.enumerate},
		&Route{verb: "GET", path: volPath("/{id}"), fn: vd.inspect},
		&Route{verb: "DELETE", path: volPath("/{id}"), fn: vd.delete},
		&Route{verb: "GET", path: volPath("/stats"), fn: vd.stats},
		&Route{verb: "GET", path: volPath("/stats/{id}"), fn: vd.stats},
		&Route{verb: "GET", path: volPath("/alerts"), fn: vd.alerts},
		&Route{verb: "GET", path: volPath("/alerts/{id}"), fn: vd.alerts},
		&Route{verb: "GET", path: volPath("/requests"), fn: vd.requests},
		&Route{verb: "GET", path: volPath("/requests/{id}"), fn: vd.requests},
		&Route{verb: "POST", path: snapPath(""), fn: vd.snap},
		&Route{verb: "GET", path: snapPath(""), fn: vd.snapEnumerate},
	}
}
