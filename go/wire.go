package main

import "encoding/json"

func (r *executorRequest) UnmarshalJSON(raw []byte) error {
	type plain executorRequest
	var decoded plain
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	*r = executorRequest(decoded)
	var lower struct {
		OriginalRequest []byte `json:"original_request"`
		HostCallbackID  string `json:"host_callback_id"`
		StreamID        string `json:"stream_id"`
	}
	if err := json.Unmarshal(raw, &lower); err != nil {
		return err
	}
	if len(r.OriginalRequest) == 0 {
		r.OriginalRequest = lower.OriginalRequest
	}
	if r.HostCallbackID == "" {
		r.HostCallbackID = lower.HostCallbackID
	}
	if r.StreamID == "" {
		r.StreamID = lower.StreamID
	}
	return nil
}
