import React from 'react';
import {
  Drawer,
  Timeline,
  Text,
  Badge,
  Paper,
  Code,
  Group,
  ThemeIcon,
  Box,
  ScrollArea,
} from '@mantine/core';
import {
  IconHistory,
  IconCheck,
  IconX,
  IconPlayerPlay,
  IconShieldCheck,
  IconSparkles,
  IconAlertTriangle,
} from '@tabler/icons-react';
import { AuditEvent } from '../types';

interface AuditDrawerProps {
  opened: boolean;
  onClose: () => void;
  events: AuditEvent[];
  isLoading: boolean;
}

export const AuditDrawer: React.FC<AuditDrawerProps> = ({
  opened,
  onClose,
  events,
  isLoading,
}) => {
  const getEventIcon = (eventType: string) => {
    switch (eventType) {
      case 'remediation_executed':
        return <IconCheck size={14} />;
      case 'remediation_approved':
        return <IconShieldCheck size={14} />;
      case 'remediation_rejected':
        return <IconX size={14} />;
      case 'recon_run':
        return <IconPlayerPlay size={14} />;
      case 'investigate':
      case 'workflow_started':
        return <IconSparkles size={14} />;
      case 'escalate':
        return <IconAlertTriangle size={14} />;
      default:
        return <IconHistory size={14} />;
    }
  };

  const getEventColor = (eventType: string) => {
    switch (eventType) {
      case 'remediation_executed':
        return 'teal';
      case 'remediation_approved':
        return 'blue';
      case 'remediation_rejected':
        return 'red';
      case 'recon_run':
        return 'indigo';
      case 'investigate':
      case 'workflow_started':
        return 'yellow';
      case 'escalate':
        return 'violet';
      default:
        return 'gray';
    }
  };

  return (
    <Drawer
      opened={opened}
      onClose={onClose}
      position="right"
      size="md"
      title={
        <Group gap="xs">
          <IconHistory size={20} color="#4c6ef5" />
          <Text size="md" fw={700}>
            Immutable Audit Trail Ledger
          </Text>
        </Group>
      }
      styles={{
        body: { padding: '16px' },
      }}
    >
      <Text size="xs" c="dimmed" mb="md">
        Append-only regulatory audit log recording all reconciliation runs, agent investigations, and operator decisions.
      </Text>

      <ScrollArea style={{ height: 'calc(100vh - 120px)' }}>
        {events.length === 0 ? (
          <Text size="sm" c="dimmed" style={{ textAlign: 'center', marginTop: 40 }}>
            No audit events recorded yet.
          </Text>
        ) : (
          <Timeline active={events.length} bulletSize={24} lineWidth={2}>
            {events.map((ev) => (
              <Timeline.Item
                key={ev.id}
                bullet={getEventIcon(ev.event_type)}
                color={getEventColor(ev.event_type)}
                title={
                  <Group justify="space-between">
                    <Text size="xs" fw={700}>
                      {ev.event_type.replace('_', ' ').toUpperCase()}
                    </Text>
                    <Text size="xs" c="dimmed">
                      {new Date(ev.created_at).toLocaleTimeString()}
                    </Text>
                  </Group>
                }
              >
                <Text size="xs" c="dimmed" mb={4}>
                  Actor: <Badge size="xs" variant="light">{ev.actor}</Badge>
                </Text>

                {ev.exception_id && (
                  <Text size="xs" c="dimmed">
                    Exception ID: <strong>#{ev.exception_id}</strong>
                  </Text>
                )}

                {ev.remediation_id && (
                  <Text size="xs" c="dimmed">
                    Remediation ID: <strong>#{ev.remediation_id}</strong>
                  </Text>
                )}

                {ev.details && Object.keys(ev.details).length > 0 && (
                  <Paper withBorder p={6} radius="xs" style={{ backgroundColor: '#f8f9fa' }} mt={6}>
                    <Code block style={{ fontSize: 10 }}>
                      {JSON.stringify(ev.details, null, 2)}
                    </Code>
                  </Paper>
                )}
              </Timeline.Item>
            ))}
          </Timeline>
        )}
      </ScrollArea>
    </Drawer>
  );
};

