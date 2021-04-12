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

import React from 'react'
import PropTypes from 'prop-types';
import { Box, ButtonSecondary, ButtonPrimary, Text } from 'shared/components';
import * as Alerts from 'shared/components/Alert';
import { useAttempt, withState } from 'shared/hooks';
import Dialog, { DialogHeader, DialogContent, DialogFooter} from 'shared/components/DialogConfirmation';
import { deleteAuthProvider } from 'e-app/cluster/flux/authConnectors/actions';

export function DeleteConnectorDialog(props){
  const { connector, onClose, onDelete } = props;
  if(!connector){
    return null;
  }

  // build state
  const [ attempt, attempActions ] = useAttempt();

  const onOk = () => {
    attempActions.do(() => onDelete(connector))
      .then(() => onClose());
  };

  const { name } = connector;
  const isDisabled = attempt.isProcessing;

  return (
    <Dialog
      disableEscapeKeyDown={false}
      onClose={onClose}
      open={true}
    >
      <Box width="540px">
        {attempt.isFailed &&  (
          <DialogHeader mb="0">
            <Alerts.Danger mb="0">
              {attempt.message}
            </Alerts.Danger>
          </DialogHeader>
        )}
        <DialogContent>
          <Text typography="h2">Remove Connector?</Text>
          <Text typography="paragraph" mt="2" mb="6">
            Are you sure you want to delete connector <Text as="span" bold color="primary.contrastText">{name}</Text>?
          </Text>
        </DialogContent>
        <DialogFooter>
          <ButtonPrimary mr="3" disabled={isDisabled} onClick={onOk}>
            DELETE
          </ButtonPrimary>
          <ButtonSecondary disabled={isDisabled} onClick={onClose}>
            Cancel
          </ButtonSecondary>
        </DialogFooter>
      </Box>
    </Dialog>
  );
}

DeleteConnectorDialog.propTypes = {
  onClose: PropTypes.func.isRequired,
  onDelete: PropTypes.func.isRequired,
  connector: PropTypes.object,
}

export default withState(() => {
  return {
    onDelete: deleteAuthProvider
  }
})(DeleteConnectorDialog);

