import React, { useState } from 'react';
import {
  Table,
  Badge,
  Button,
  Group,
  Text,
  Paper,
  TextInput,
  Select,
  ActionIcon,
  Tooltip,
  Box,
} from '@mantine/core';
import {
  IconSearch,
  IconSparkles,
  IconArrowUpRight,
  IconShieldExclamation,
  IconFilter,
  IconCheck,
} from '@tabler/icons-react';
import { Exception, ExceptionSeverity, ExceptionStatus, UserRole } from '../types';
import { formatINR } from '../utils/money';

interface ExceptionTableProps {
  exceptions: Exception[];
  onInvestigate: (id: number) => void;
  investigatingId: number | null;
  onSelectException: (exception: Exception) => void;
  onEscalate: (id: number) => void;
  currentRole: UserRole;
}

const severityColors: Record<ExceptionSeverity, string> = {
  high: 'red',
  medium: 'orange',
  low: 'blue',
};

const statusColors: Record<ExceptionStatus, string> = {
  open: 'red',
  investigating: 'yellow',
  proposal_ready: 'indigo',
  action_taken: 'teal',
  escalated: 'violet',
  resolved: 'gray',
};

export const ExceptionTable: React.FC<ExceptionTableProps> = ({
  exceptions,
  onInvestigate,
  investigatingId,
  onSelectException,
  onEscalate,
  currentRole,
}) => {
  const [searchTerm, setSearchTerm] = useState('');
  const [statusFilter, setStatusFilter] = useState<string>('all');
  const [typeFilter, setTypeFilter] = useState<string>('all');

  const filtered = exceptions.filter((exc) => {
    const matchesSearch =
      exc.batch_ref.toLowerCase().includes(searchTerm.toLowerCase()) ||
      exc.type.toLowerCase().includes(searchTerm.toLowerCase()) ||
      exc.reason_code.toLowerCase().includes(searchTerm.toLowerCase());

    const matchesStatus = statusFilter === 'all' || exc.status === statusFilter;
    const matchesType = typeFilter === 'all' || exc.type === typeFilter;

    return matchesSearch && matchesStatus && matchesType;
  });

  return (
    <Paper withBorder p="md" radius="md" mb="lg">
      <Group justify="space-between" mb="md">
        <Box>
          <Text size="md" fw={700}>
            Reconciliation Exceptions
          </Text>
          <Text size="xs" c="dimmed">
            Deterministic variances flagged between gateway batch settlements, bank receipts, and accounting
          </Text>
        </Box>

        {/* Filter Controls */}
        <Group gap="xs">
          <TextInput
            size="xs"
            placeholder="Search ref or reason..."
            leftSection={<IconSearch size={14} />}
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.currentTarget.value)}
            style={{ width: 180 }}
          />

          <Select
            size="xs"
            value={typeFilter}
            onChange={(v) => setTypeFilter(v || 'all')}
            data={[
              { label: 'All Types', value: 'all' },
              { label: 'FEE_VARIANCE', value: 'FEE_VARIANCE' },
              { label: 'DELAYED_OR_SPLIT_DEPOSIT', value: 'DELAYED_OR_SPLIT_DEPOSIT' },
              { label: 'REFUND_NOT_IN_LEDGER', value: 'REFUND_NOT_IN_LEDGER' },
              { label: 'MISSING_DEPOSIT', value: 'MISSING_DEPOSIT' },
            ]}
            style={{ width: 180 }}
          />

          <Select
            size="xs"
            value={statusFilter}
            onChange={(v) => setStatusFilter(v || 'all')}
            data={[
              { label: 'All Statuses', value: 'all' },
              { label: 'Open', value: 'open' },
              { label: 'Proposal Ready', value: 'proposal_ready' },
              { label: 'Action Taken', value: 'action_taken' },
              { label: 'Escalated', value: 'escalated' },
            ]}
            style={{ width: 140 }}
          />
        </Group>
      </Group>

      {/* Table */}
      <Table.ScrollContainer minWidth={800}>
        <Table verticalSpacing="sm" highlightOnHover>
          <Table.Thead>
            <Table.Tr>
              <Table.Th>ID</Table.Th>
              <Table.Th>Batch Reference</Table.Th>
              <Table.Th>Exception Type</Table.Th>
              <Table.Th>Variance</Table.Th>
              <Table.Th>Severity</Table.Th>
              <Table.Th>Reason Code</Table.Th>
              <Table.Th>Status</Table.Th>
              <Table.Th style={{ textAlign: 'right' }}>Actions</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {filtered.length === 0 ? (
              <Table.Tr>
                <Table.Td colSpan={8} style={{ textAlign: 'center', padding: '30px' }}>
                  <Text size="sm" c="dimmed">
                    No exceptions found matching filters.
                  </Text>
                </Table.Td>
              </Table.Tr>
            ) : (
              filtered.map((exc, index) => {
                const isInvestigating = investigatingId === exc.id || exc.status === 'investigating';
                const hasProposal = exc.status === 'proposal_ready';
                const isCompleted = exc.status === 'action_taken' || exc.status === 'resolved';
                const isEscalated = exc.status === 'escalated';

                return (
                  <Table.Tr key={exc.id}>
                    <Table.Td>
                      <Tooltip label={`Database Primary Key: #${exc.id}`}>
                        <Text size="xs" c="dimmed" fw={600} style={{ cursor: 'help' }}>
                          #{index + 1}
                        </Text>
                      </Tooltip>
                    </Table.Td>
                    <Table.Td>
                      <Text size="sm" fw={600} ff="monospace">
                        {exc.batch_ref}
                      </Text>
                    </Table.Td>
                    <Table.Td>
                      <Badge variant="light" color="dark">
                        {exc.type}
                      </Badge>
                    </Table.Td>
                    <Table.Td>
                      <Text size="sm" fw={700} c={exc.variance_minor < 0 ? 'red.7' : 'teal.7'}>
                        {formatINR(exc.variance_minor)}
                      </Text>
                    </Table.Td>
                    <Table.Td>
                      <Badge color={severityColors[exc.severity]} size="sm">
                        {exc.severity}
                      </Badge>
                    </Table.Td>
                    <Table.Td>
                      <Text size="xs" ff="monospace" c="dimmed">
                        {exc.reason_code}
                      </Text>
                    </Table.Td>
                    <Table.Td>
                      <Badge color={statusColors[exc.status]} variant="dot">
                        {exc.status.replace('_', ' ')}
                      </Badge>
                    </Table.Td>
                    <Table.Td style={{ textAlign: 'right' }}>
                      <Group gap="xs" justify="flex-end">
                        {/* Investigate / Re-investigate Action */}
                        {!isCompleted && (
                          <Tooltip
                            label={
                              currentRole === 'viewer'
                                ? 'Viewer role is read-only'
                                : isEscalated
                                ? 'Re-run bounded AI investigation on this escalated exception'
                                : 'Launch bounded AI agent investigation'
                            }
                          >
                            <Button
                              size="xs"
                              variant={isEscalated ? 'light' : 'filled'}
                              color={isEscalated ? 'violet' : 'indigo'}
                              leftSection={<IconSparkles size={14} />}
                              loading={isInvestigating}
                              disabled={currentRole === 'viewer' || isInvestigating}
                              onClick={() => onInvestigate(exc.id)}
                            >
                              {isEscalated ? 'Re-investigate' : 'Investigate'}
                            </Button>
                          </Tooltip>
                        )}

                        {/* View Drawer Button */}
                        <Tooltip label="Inspect variance breakdown, tool execution trace, & remediation">
                          <Button
                            size="xs"
                            variant="light"
                            color="gray"
                            leftSection={<IconArrowUpRight size={14} />}
                            onClick={() => onSelectException(exc)}
                          >
                            {hasProposal ? 'Review Draft' : isCompleted ? 'Audit Trail' : isEscalated ? 'View Escalation' : 'Inspect'}
                          </Button>
                        </Tooltip>

                        {/* Escalate Button */}
                        {exc.status === 'open' && (
                          <Tooltip
                            label={
                              currentRole === 'viewer'
                                ? 'Viewer role is read-only'
                                : 'Escalate directly to Treasury Operations'
                            }
                          >
                            <ActionIcon
                              size="sm"
                              variant="subtle"
                              color="violet"
                              disabled={currentRole === 'viewer'}
                              onClick={() => onEscalate(exc.id)}
                            >
                              <IconShieldExclamation size={16} />
                            </ActionIcon>
                          </Tooltip>
                        )}
                      </Group>
                    </Table.Td>
                  </Table.Tr>
                );
              })
            )}
          </Table.Tbody>
        </Table>
      </Table.ScrollContainer>
    </Paper>
  );
};

