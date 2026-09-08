import React from 'react';
import { AccessContext, WorkspaceDefinition } from '../platform/navigation';
import { WorkspaceHost } from './WorkspaceHost';

interface WorkspacePageProps {
  workspace: WorkspaceDefinition;
  access: AccessContext;
  onNavigate: (workspace: WorkspaceDefinition) => void;
}

/** Thin route adapter; all workspace chrome lives in WorkspaceHost. */
export const WorkspacePage: React.FC<WorkspacePageProps> = ({
  workspace,
  access,
  onNavigate,
}) => (
  <WorkspaceHost workspace={workspace} access={access} onNavigate={onNavigate} />
);
