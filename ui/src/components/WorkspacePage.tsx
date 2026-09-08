import React from 'react';
import { AccessContext, WorkspaceDefinition } from '../platform/navigation';
import { WorkspaceHost } from './WorkspaceHost';

interface WorkspacePageProps {
  workspace: WorkspaceDefinition;
  access: AccessContext;
  onNavigate: (workspace: WorkspaceDefinition) => void;
}

/** Thin route adapter; remount host on workspace id change to clear tab state. */
export const WorkspacePage: React.FC<WorkspacePageProps> = ({
  workspace,
  access,
  onNavigate,
}) => (
  <WorkspaceHost
    key={workspace.id}
    workspace={workspace}
    access={access}
    onNavigate={onNavigate}
  />
);
