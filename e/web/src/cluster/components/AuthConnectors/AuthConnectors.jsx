/**
 * Copyright 2021 Gravitational Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import React from 'react';
import { useFluxStore } from 'oss-app/components/nuclear';
import userAclGetters from 'oss-app/flux/userAcl/getters';
import ResourceEditor from 'oss-app/components/ResourceEditor';
import { FeatureBox, FeatureHeader, FeatureHeaderTitle } from 'oss-app/cluster/components/Layout';
import * as authFlux from 'e-app/cluster/flux/authConnectors';
import * as actions from 'e-app/cluster/flux/authConnectors/actions';
import { Text, Box, Flex  } from 'shared/components';
import { useState, withState } from 'shared/hooks';
import AddMenu from './AddMenu';
import { getTemplate } from './templates';
import EmptyList from './EmptyList';
import ConnectorList from './ConnectorList';
import DeleteConnectorDialog from './DeleteConnectorDialog';

export function AuthConnectors(props){
  const { store, userAclStore } = props;
  const [resource, { onCreate, onEdit, onSave, onClose } ] = useResourceEditor(props);
  const [ connToDelete, setConnToDelete ] = useState(null);

  const onDelete = id => {
    const selectedConn = store.findItem(id);
    setConnToDelete(selectedConn);
  }

  const items = store.getItems();
  const access = userAclStore.getConnectorAccess();
  const canCreate = access.create;

  const isNewConnector = resource && resource.isNew;
  const isEmpty = items.size === 0;
  const resourceEditorTitle = isNewConnector ? "Creating a new auth connector" : "Editing auth connector";

  return (
    <FeatureBox>
      <FeatureHeader>
        <FeatureHeaderTitle>
          Auth Connectors
        </FeatureHeaderTitle>
        <Box ml="auto" alignSelf="center" width="240px">
          <AddMenu onClick={onCreate} disabled={!canCreate}/>
        </Box>
      </FeatureHeader>
      <Flex alignItems="start">
        {isEmpty && (
          <Flex mt="6" width="100%" justifyContent="center">
            <EmptyList onCreate={onCreate}/>
          </Flex>
        )}
        {!isEmpty && (
          <ConnectorList flex="1" items={items} onEdit={onEdit} onDelete={onDelete} />
        )}
        <Box ml="4" width="240px" color="text.primary" style={{flexShrink: 0}}>
          <Text typography="h6" mb={3}>
            AUTHENTICATION CONNECTORS
          </Text>
          <Text typography="subtitle1" mb={3}>
            Authentication connectors allow Gravity to authenticate users via an external identity source such as Okta,
            Active Directory, Github, etc. This authentication method is frequenty called single sign-on (SSO).
          </Text>
          <Text typography="subtitle1" mb={2}>
            Please <Text as="a" color="light" href="https://gravitational.com/gravity/docs/cluster/#configuring-openid-connect" target="_blank">view our documentation</Text> for samples of each connector.
          </Text>
        </Box >
      </Flex>
      <ResourceEditor onSave={onSave}
        title={resourceEditorTitle}
        onClose={onClose}
        resource={resource}
      />
      <DeleteConnectorDialog
        connector={connToDelete}
        onClose={() => setConnToDelete(null)}
       />
      </FeatureBox>
  );
}

/**
 * Resource Editor state
 */
function useResourceEditor(props){
  const { store, saveConnector } = props;
  const [ resource, setResource ] = useState(null);

  const onCreate = kind => {
    const content = getTemplate(kind);
    setResource({
      isNew: true,
      content,
    })
  }

  const onClose = () => {
    setResource(null)
  }

  const onEdit = id => {
    const { content, name } = store.findItem(id);
    setResource({
      content,
      name,
    });
  }

  const onSave = resource => {
    return saveConnector(resource.content, resource.isNew);
  }

  return [resource, { onCreate, onEdit, onClose, onSave }];
}

function mapState() {
  const store = useFluxStore(authFlux.getters.store);
  const userAclStore = useFluxStore(userAclGetters.userAcl);
  return {
    store,
    userAclStore,
    saveConnector: actions.saveAuthProvider,
    deleteConnector: actions.deleteAuthProvider
  }
}

export default withState(mapState)(AuthConnectors);