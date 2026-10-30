import React, { useState } from 'react';
import {
  Group,
  Title,
  Badge,
  Button,
  Select,
  ActionIcon,
  Tooltip,
  Paper,
  Box,
  Text,
} from '@mantine/core';
import {
  IconDatabaseImport,
  IconPlayerPlay,
  IconHistory,
  IconCheck,
  IconUserCheck,
  IconEye,
  IconHeartbeat,
} from '@tabler/icons-react';
import { UserRole } from '../types';
import { getActiveRole, setActiveRole } from '../api/client';

interface HeaderProps {
  onLoadSeed: () => void;
  isLoadingSeed: boolean;
  onRunRecon: (asOf: string) => void;
  isRunningRecon: boolean;
  onOpenAudit: () => void;
  onRoleChange: (role: UserRole) => void;
  systemHealth: { status: string; db: string; goose_version: number } | null;
}

export const Header: React.FC<HeaderProps> = ({
  onLoadSeed,
  isLoadingSeed,
  onRunRecon,
  isRunningRecon,
  onOpenAudit,
  onRoleChange,
  systemHealth,
}) => {
  const [role, setRole] = useState<UserRole>(getActiveRole());
  const [asOfDate, setAsOfDate] = useState<string>('2026-10-20');

  const handleRoleToggle = (newRole: UserRole) => {
    setRole(newRole);
    setActiveRole(newRole);
    onRoleChange(newRole);
  };

  const isDBConnected = systemHealth?.db === 'connected';

  return (
    <Paper shadow="xs" p="md" withBorder mb="lg" style={{ backgroundColor: '#ffffff' }}>
      <Group justify="space-between" align="center">
        {/* Brand & System Health */}
        <Group gap="sm">
          <Box
            style={{
              width: 38,
              height: 38,
              borderRadius: 8,
              backgroundColor: '#1c7ed6',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: '#ffffff',
              fontSize: 20,
              fontWeight: 700,
            }}
          >
            ⚖️
          </Box>
          <Box>
            <Group gap="xs">
              <Title order={3} style={{ fontWeight: 700, letterSpacing: -0.5 }}>
                Payment Settlement Investigation Workstation
              </Title>
              <Badge
                color={isDBConnected ? 'teal' : 'red'}
                variant="light"
                leftSection={<IconHeartbeat size={14} />}
              >
                {isDBConnected ? 'DB Connected' : 'DB Offline'}
              </Badge>
            </Group>
            <Text size="xs" c="dimmed">
              Autonomous financial reconciliation & human-in-the-loop exception resolution
            </Text>
          </Box>
        </Group>

        {/* Global Controls */}
        <Group gap="sm">
          {/* As-Of Date Selector */}
          <Select
            size="xs"
            label="As-Of Date"
            data={['2026-10-20', '2026-10-15', '2026-10-14']}
            value={asOfDate}
            onChange={(val) => setAsOfDate(val || '2026-10-20')}
            style={{ width: 130 }}
          />

          {/* Load Seed Data */}
          <Tooltip label="Import deterministic seed files (orders, payments, bank deposits, schedules)">
            <Button
              size="xs"
              variant="default"
              leftSection={<IconDatabaseImport size={15} />}
              loading={isLoadingSeed}
              onClick={onLoadSeed}
              disabled={role === 'viewer'}
            >
              Load Seed Data
            </Button>
          </Tooltip>

          {/* Run Reconciliation */}
          <Tooltip label={`Run deterministic reconciliation engine as of ${asOfDate}`}>
            <Button
              size="xs"
              color="indigo"
              leftSection={<IconPlayerPlay size={15} />}
              loading={isRunningRecon}
              onClick={() => onRunRecon(asOfDate)}
              disabled={role === 'viewer'}
            >
              Reconcile
            </Button>
          </Tooltip>

          {/* Audit Trail Drawer Toggle */}
          <Tooltip label="View immutable append-only audit trail">
            <Button
              size="xs"
              variant="light"
              color="gray"
              leftSection={<IconHistory size={15} />}
              onClick={onOpenAudit}
            >
              Audit Log
            </Button>
          </Tooltip>

          {/* Role Toggle Switcher */}
          <Paper withBorder p={3} radius="md" style={{ backgroundColor: '#f8f9fa' }}>
            <Group gap={4}>
              <Tooltip label="Operator role: Full authority to investigate, approve, and execute remediations">
                <Button
                  size="compact-xs"
                  variant={role === 'operator' ? 'filled' : 'subtle'}
                  color="teal"
                  leftSection={<IconUserCheck size={13} />}
                  onClick={() => handleRoleToggle('operator')}
                >
                  Operator
                </Button>
              </Tooltip>
              <Tooltip label="Viewer role: Read-only access (financial state changes return 403 Forbidden)">
                <Button
                  size="compact-xs"
                  variant={role === 'viewer' ? 'filled' : 'subtle'}
                  color="gray"
                  leftSection={<IconEye size={13} />}
                  onClick={() => handleRoleToggle('viewer')}
                >
                  Viewer
                </Button>
              </Tooltip>
            </Group>
          </Paper>
        </Group>
      </Group>
    </Paper>
  );
};

