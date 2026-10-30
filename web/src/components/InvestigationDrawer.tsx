import React from 'react';
import { Drawer, Grid, Box, Title, Text, Group, Badge, ActionIcon } from '@mantine/core';
import { IconX } from '@tabler/icons-react';
import { Exception, WorkflowDetail, Remediation, UserRole } from '../types';
import { VarianceBreakdown } from './VarianceBreakdown';
import { AgentTrace } from './AgentTrace';

interface InvestigationDrawerProps {
  opened: boolean;
  onClose: () => void;
  exception: Exception | null;
  workflowDetail: WorkflowDetail | null;
  isLoadingWorkflow: boolean;
  remediation?: Remediation;
  onApproveRemediation: (id: number) => void;
  isApproving: boolean;
  onRejectRemediation: (id: number, note?: string) => void;
  isRejecting: boolean;
  onRetryRemediation: (id: number) => void;
  isRetrying: boolean;
  currentRole: UserRole;
}

export const InvestigationDrawer: React.FC<InvestigationDrawerProps> = ({
  opened,
  onClose,
  exception,
  workflowDetail,
  isLoadingWorkflow,
  remediation,
  onApproveRemediation,
  isApproving,
  onRejectRemediation,
  isRejecting,
  onRetryRemediation,
  isRetrying,
  currentRole,
}) => {
  if (!exception) return null;

  return (
    <Drawer
      opened={opened}
      onClose={onClose}
      position="right"
      size="85%"
      title={
        <Group gap="xs">
          <Title order={4}>Investigation Workspace: {exception.batch_ref}</Title>
          <Badge color="dark" size="sm">
            {exception.type}
          </Badge>
        </Group>
      }
      styles={{
        body: { padding: '16px', backgroundColor: '#fdfdfe' },
        header: { borderBottom: '1px solid #dee2e6' },
      }}
    >
      <Grid gutter="lg">
        {/* Left Column (5/12): Variance Decomposition & Mathematical Evidence */}
        <Grid.Col span={{ base: 12, md: 5 }}>
          <VarianceBreakdown exception={exception} />
        </Grid.Col>

        {/* Right Column (7/12): Autonomous Agent Execution Trace & Remediation Card */}
        <Grid.Col span={{ base: 12, md: 7 }}>
          <AgentTrace
            workflowDetail={workflowDetail}
            isLoading={isLoadingWorkflow}
            remediation={remediation || exception.remediation}
            onApproveRemediation={onApproveRemediation}
            isApproving={isApproving}
            onRejectRemediation={onRejectRemediation}
            isRejecting={isRejecting}
            onRetryRemediation={onRetryRemediation}
            isRetrying={isRetrying}
            currentRole={currentRole}
          />
        </Grid.Col>
      </Grid>
    </Drawer>
  );
};

