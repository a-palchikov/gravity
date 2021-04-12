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

import React from "react";
import PropTypes from "prop-types";
import styled from "styled-components";
import { NavLink } from "react-router-dom";
import { Redirect, Switch, Route } from "oss-app/components/Router";
import { FeatureHeader as OssFeatureHeader } from "oss-app/cluster/components/Layout";
import { SideNav, SideNavItem } from "shared/components";
import SideNavItemIcon from "shared/components/SideNav/SideNavItemIcon";
import { FeatureBox } from "../Layout";

export default function SideNavLayout({ navItems, feature }) {
  const $items = navItems.map((item, index) => (
    <SideNavItem key={index} as={NavLink} exact={item.exact} to={item.to}>
      <SideNavItemIcon as={item.Icon} />
      {item.title}
    </SideNavItem>
  ));

  // when hitting the index route, be ready to redirect to
  // the first available nav item
  const indexRoute = feature.path;
  const indexTab = navItems.length > 0 ? navItems[0].to : null;

  const features = feature.features;

  return (
    <StyledFeatureBox>
      <SideNav>{$items}</SideNav>
      <Switch>
        {indexTab && <Redirect exact from={indexRoute} to={indexTab} />}
        {renderFeatures(features)}
      </Switch>
    </StyledFeatureBox>
  );
}

const StyledFeatureBox = styled(FeatureBox)`
  padding-left: 0;
  padding-right: 0;
  flex-direction: row;
  ${OssFeatureHeader} {
    margin-top: ${({ theme }) => `${theme.space[4]}px`};
  }
`;

function renderFeatures(features) {
  if (!features) {
    return null;
  }

  const $features = features.map((item, index) => {
    const { path, title, exact, component } = item.getRoute();
    return (
      <Route
        title={title}
        key={index}
        path={path}
        exact={exact}
        component={component}
      />
    );
  });

  return $features;
}

SideNavLayout.propTypes = {
  feature: PropTypes.object.isRequired,
  navItems: PropTypes.array.isRequired
};
