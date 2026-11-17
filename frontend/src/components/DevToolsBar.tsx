import React, { useState } from 'react';
import {
  Paper,
  Group,
  Text,
  Badge,
  SegmentedControl,
  Switch,
  Tooltip,
  Box,
  Button,
} from '@mantine/core';
import { IconCpu, IconBug, IconLock, IconRotate2 } from '@tabler/icons-react';
import { MockFailureMode, UserRole } from '../types';
import { api } from '../api/client';

interface DevToolsBarProps {
  currentRole: UserRole;
  onRoleChange: (role: UserRole) => void;
  syncInvestigation: boolean;
  onSyncChange: (sync: boolean) => void;
}

export const DevToolsBar: React.FC<DevToolsBarProps> = ({
  currentRole,
  onRoleChange,
  syncInvestigation,
  onSyncChange,
}) => {
  const [failureMode, setFailureMode] = useState<MockFailureMode['mode']>('none');
  const [isInjecting, setIsInjecting] = useState<boolean>(false);

  const [isResetting, setIsResetting] = useState<boolean>(false);

  const handleResetDemo = async () => {
    if (!window.confirm('Reset all reconciliation data and reload seed files?')) return;
    setIsResetting(true);
    try {
      await api.resetDemoData();
      window.location.reload();
    } catch (err) {
      console.error('Reset failed:', err);
    } finally {
      setIsResetting(false);
    }
  };

  const handleFailureModeChange = async (val: string) => {
    const mode = val as MockFailureMode['mode'];
    setIsInjecting(true);
    try {
      await api.setMockFailureMode(mode);
      setFailureMode(mode);
    } catch (err) {
      console.error('Failed to set mock failure mode:', err);
    } finally {
      setIsInjecting(false);
    }
  };

  return (
    <Paper
      withBorder
      p="xs"
      mb="md"
      radius="sm"
      style={{
        backgroundColor: '#f1f3f5',
        borderLeft: '4px solid #495057',
      }}
    >
      <Group justify="space-between" align="center">
        <Group gap="sm">
          <Group gap={6}>
            <IconCpu size={16} color="#495057" />
            <Text size="xs" fw={700} c="dimmed" tt="uppercase">
              Dev & Simulation Controls:
            </Text>
          </Group>

          {/* Role Status Indicator */}
          <Badge
            size="sm"
            color={currentRole === 'operator' ? 'teal' : 'gray'}
            variant="dot"
          >
            Acting as: {currentRole === 'operator' ? 'operator:Alice' : 'viewer:Auditor'}
          </Badge>

          {/* Mock External Failure Injection */}
          <Group gap={6}>
            <IconBug size={15} color="#e03131" />
            <Text size="xs" fw={600}>
              Gateway/Accounting Mock Failure:
            </Text>
            <SegmentedControl
              size="xs"
              value={failureMode}
              onChange={handleFailureModeChange}
              disabled={isInjecting || currentRole === 'viewer'}
              data={[
                { label: 'Normal (None)', value: 'none' },
                { label: 'HTTP 500 Error', value: 'error_500' },
                { label: 'Commit then 500', value: 'commit_then_500' },
              ]}
            />
          </Group>
        </Group>

        <Group gap="md">
          {/* Synchronous vs Background Async Execution */}
          <Tooltip label="Synchronous waits for full agent loop (3-4s); Asynchronous returns immediately and polls background run.">
            <Switch
              size="xs"
              label="Sync Investigation"
              checked={syncInvestigation}
              onChange={(e) => onSyncChange(e.currentTarget.checked)}
            />
          </Tooltip>

          {/* Reset Demo Button */}
          <Tooltip label="Wipe all ledger runs and reset database back to initial state">
            <Button
              size="xs"
              variant="subtle"
              color="gray"
              leftSection={<IconRotate2 size={13} />}
              loading={isResetting}
              onClick={handleResetDemo}
            >
              Reset Demo
            </Button>
          </Tooltip>

          {currentRole === 'viewer' && (
            <Badge color="red" size="xs" variant="light" leftSection={<IconLock size={12} />}>
              RBAC Enforced (Viewer Read-Only)
            </Badge>
          )}
        </Group>
      </Group>
    </Paper>
  );
};

