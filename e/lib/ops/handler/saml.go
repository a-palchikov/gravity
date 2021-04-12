// Copyright 2021 Gravitational Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gravitational/gravity/lib/constants"
	"github.com/gravitational/gravity/lib/ops/opsclient"

	"github.com/gravitational/roundtrip"
	"github.com/gravitational/teleport/lib/httplib"
	"github.com/gravitational/teleport/lib/services"
	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
	"github.com/julienschmidt/httprouter"
)

/* upsertSAMLConnector creates or updates SAML connector

   POST /portal/v1/accounts/:account_id/sites/:site_domain/saml/connectors
*/
func (h *WebHandler) upsertSAMLConnector(w http.ResponseWriter, r *http.Request, p httprouter.Params, ctx *handlerContext) error {
	var req *opsclient.UpsertResourceRawReq
	if err := httplib.ReadJSON(r, &req); err != nil {
		return trace.Wrap(err)
	}
	connector, err := services.GetSAMLConnectorMarshaler().UnmarshalSAMLConnector(req.Resource)
	if err != nil {
		return trace.Wrap(err)
	}
	if req.TTL != 0 {
		connector.SetTTL(clockwork.NewRealClock(), req.TTL)
	}
	err = ctx.Operator.UpsertSAMLConnector(r.Context(), siteKey(p), connector)
	if err != nil {
		return trace.Wrap(err)
	}
	roundtrip.ReplyJSON(w, http.StatusOK, message("connector configuration applied"))
	return nil
}

/* getSAMLConnector returns a SAML connector by name

   GET /portal/v1/accounts/:account_id/sites/:site_domain/saml/connectors/:id
*/
func (h *WebHandler) getSAMLConnector(w http.ResponseWriter, r *http.Request, p httprouter.Params, ctx *handlerContext) error {
	withSecrets, _, err := httplib.ParseBool(r.URL.Query(), constants.WithSecretsParam)
	if err != nil {
		return trace.Wrap(err)
	}
	connector, err := ctx.Operator.GetSAMLConnector(siteKey(p), p.ByName("id"), withSecrets)
	if err != nil {
		return trace.Wrap(err)
	}
	out, err := services.GetSAMLConnectorMarshaler().MarshalSAMLConnector(connector)
	return rawMessage(w, out, err)
}

/* getSAMLConnectors returns all SAML connectors

   GET /portal/v1/accounts/:account_id/sites/:site_domain/saml/connectors
*/
func (h *WebHandler) getSAMLConnectors(w http.ResponseWriter, r *http.Request, p httprouter.Params, ctx *handlerContext) error {
	withSecrets, _, err := httplib.ParseBool(r.URL.Query(), constants.WithSecretsParam)
	if err != nil {
		return trace.Wrap(err)
	}
	connectors, err := ctx.Operator.GetSAMLConnectors(siteKey(p), withSecrets)
	if err != nil {
		return trace.Wrap(err)
	}
	items := make([]json.RawMessage, len(connectors))
	for i, connector := range connectors {
		data, err := services.GetSAMLConnectorMarshaler().MarshalSAMLConnector(connector)
		if err != nil {
			return trace.Wrap(err)
		}
		items[i] = data
	}
	roundtrip.ReplyJSON(w, http.StatusOK, items)
	return nil
}

/* deleteSAMLConnector deletes a SAML connector by name

   DELETE /portal/v1/accounts/:account_id/sites/:site_domain/saml/connectors/:id
*/
func (h *WebHandler) deleteSAMLConnector(w http.ResponseWriter, r *http.Request, p httprouter.Params, ctx *handlerContext) error {
	name := p.ByName("id")
	err := ctx.Operator.DeleteSAMLConnector(r.Context(), siteKey(p), name)
	if err != nil {
		if trace.IsNotFound(err) {
			return trace.NotFound("SAML connector %q not found", name)
		}
		return trace.Wrap(err)
	}
	roundtrip.ReplyJSON(w, http.StatusOK, message("SAML connector deleted"))
	return nil
}
