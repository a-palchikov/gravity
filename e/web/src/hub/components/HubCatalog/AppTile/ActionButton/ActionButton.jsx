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
import styled from 'styled-components';
import * as Icons from 'shared/components/Icon';
import Menu, { MenuItem} from 'shared/components/Menu';
import { Flex, ButtonPrimary } from 'shared/components';

class ActionButton extends React.Component {

  static displayName = 'ActionButton';

  constructor(props){
    super(props)
    this.state = {
      open: Boolean(props.open),
      anchorEl: null,
    }
  }

  onOpen = () => {
    this.setState({ open: true });
  };

  onClose = () => {
    this.setState({ open: false });
  }

  onItemClick = kind => {
    this.onClose();
    this.props.onClick(kind);
  }

  setRef = e => {
    this.anchorEl = e;
  }

  render() {
    const { open } = this.state;
    const { disabled, btnText, buttonProps, children, ...styles } = this.props;
    return (
      <React.Fragment>
        <Flex {...styles}>
          <ButtonPrimary width="100%"
            style={{
              "borderRadius": "4px 0px 0px 4px"
            }}
            {...buttonProps}
          >
            {btnText}
          </ButtonPrimary>
          <ToggleButton
            setRef={this.setRef}
            px="4"
            style={{
              "borderRadius": "0 4px 4px 0"
            }}
            width="8px"
            disabled={disabled}
            onClick={this.onOpen}
          >
            <Icons.CarrotDown fontSize="3" color="text.onDark"/>
          </ToggleButton>
        </Flex>
        <Menu
          anchorEl={this.anchorEl}
          open={open}
          onClose={this.onClose}
          menuListCss={menuListCss}
          anchorOrigin={{
            vertical: 'top',
            horizontal: 'right',
          }}
          transformOrigin={{
            vertical: 'top',
            horizontal: 'right',
          }}
        >
        {open && this.renderItems(children)}
        </Menu>
      </React.Fragment>
    );
  }

  renderItems(children) {
    const filtered = React.Children.toArray(children);
    const cloned = filtered.map(child => {
      return React.cloneElement(child, {
        onClick: this.makeOnClick(child.props.onClick)
      });
    })

    return cloned;
  }

  makeOnClick(cb){
    return e => {
      e.stopPropagation();
      this.onClose();
      cb && cb(e);
    }
  }
}

const ToggleButton = styled(ButtonPrimary)`
  background-color: ${ ({theme}) => theme.colors.secondary.dark};
`

const menuListCss = ({theme}) => `
  width: 210px;
  background-color: ${theme.colors.secondary.light}

  ${MenuItem} {
    padding-left: 36px;
    background-color: ${theme.colors.secondary.main};
    color: ${theme.colors.secondary.contrastText};
    &:hover,&:focus {
      background-color: ${theme.colors.secondary.light};
    }
  }
`

export default ActionButton;
export {
  MenuItem
}