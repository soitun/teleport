/**
 * Teleport
 * Copyright (C) 2024  Gravitational, Inc.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

import {
  RequiredDiscoverProviders,
  resourceSpecAppAwsCliConsole,
} from 'teleport/Discover/Fixtures/fixtures';

import { app } from '../fixtures';
import { TestConnection as Comp } from './TestConnection';

export default {
  title: 'Teleport/Discover/Application/AwsConsole',
};

export const TestConnection = () => (
  <Provider>
    <Comp />
  </Provider>
);

const Provider = ({ children }) => {
  return (
    <RequiredDiscoverProviders
      resourceSpec={resourceSpecAppAwsCliConsole}
      agentMeta={{
        app: {
          ...app,
          awsRoles: [
            {
              name: 'static-arn1',
              arn: 'arn:aws:iam::123456789012:role/static-arn1',
              display: 'static-arn1',
              accountId: '123456789012',
            },
            {
              name: 'static-arn2',
              arn: 'arn:aws:iam::123456789012:role/static-arn2',
              display: 'static-arn2',
              accountId: '123456789012',
            },
          ],
        },
      }}
    >
      {children}
    </RequiredDiscoverProviders>
  );
};
