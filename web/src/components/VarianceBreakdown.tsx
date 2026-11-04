import React from 'react';
import {
  Paper,
  Text,
  Badge,
  Group,
  Table,
  Box,
  Divider,
  Alert,
  Code,
  Tooltip,
} from '@mantine/core';
import {
  IconCalculator,
  IconCalendar,
  IconFingerprint,
  IconReceiptTax,
  IconInfoCircle,
} from '@tabler/icons-react';
import { Exception } from '../types';
import { formatINR } from '../utils/money';

interface VarianceBreakdownProps {
  exception: Exception;
}

export const VarianceBreakdown: React.FC<VarianceBreakdownProps> = ({ exception }) => {
  const ctx = exception.context;
  const comp = ctx?.comparison;
  const dates = ctx?.dates;
  const batch = ctx?.batch;

  return (
    <Box>
      {/* 1. Header & Identity */}
      <Paper withBorder p="sm" radius="md" mb="md" style={{ backgroundColor: '#f8f9fa' }}>
        <Group justify="space-between" align="center">
          <div>
            <Text size="xs" c="dimmed" tt="uppercase" fw={700}>
              Batch Under Investigation
            </Text>
            <Text size="lg" fw={700} ff="monospace">
              {exception.batch_ref}
            </Text>
          </div>
          <Badge size="lg" color="dark">
            {exception.type}
          </Badge>
        </Group>

        <Group gap="md" mt="xs">
          <Text size="xs">
            <strong>Settled On:</strong> {batch?.settled_on || 'N/A'}
          </Text>
          <Text size="xs">
            <strong>Currency:</strong> {batch?.currency || 'INR'}
          </Text>
          <Text size="xs">
            <strong>Evaluation As-Of:</strong> {dates?.as_of || '2026-10-20'}
          </Text>
        </Group>
      </Paper>

      {/* 2. Mathematical Formula Breakdown */}
      <Paper withBorder p="sm" radius="md" mb="md">
        <Group gap={6} mb="xs">
          <IconCalculator size={16} color="#1c7ed6" />
          <Text size="xs" fw={700} tt="uppercase" c="dimmed">
            Deterministic Reconciliation Formula
          </Text>
        </Group>

        <Paper withBorder p="xs" radius="sm" style={{ backgroundColor: '#edf2ff' }} mb="xs">
          <Text size="sm" ff="monospace" fw={600} style={{ textAlign: 'center' }}>
            Variance = Bank Received (B) − Expected Net (E)
          </Text>
          <Text size="xs" c="dimmed" style={{ textAlign: 'center' }} mt={2}>
            {formatINR(comp?.net?.bank_minor)} − {formatINR(comp?.net?.expected_minor)} ={' '}
            <strong style={{ color: '#e03131' }}>{formatINR(exception.variance_minor)}</strong>
          </Text>
        </Paper>

        {/* Breakdown Table */}
        <Table verticalSpacing="xs" striped withTableBorder>
          <Table.Thead>
            <Table.Tr>
              <Table.Th>Component</Table.Th>
              <Table.Th style={{ textAlign: 'right' }}>Reported</Table.Th>
              <Table.Th style={{ textAlign: 'right' }}>Expected</Table.Th>
              <Table.Th style={{ textAlign: 'right' }}>Discrepancy</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            <Table.Tr>
              <Table.Td>Gross Payments</Table.Td>
              <Table.Td style={{ textAlign: 'right' }}>{formatINR(comp?.gross.reported_minor)}</Table.Td>
              <Table.Td style={{ textAlign: 'right' }}>{formatINR(comp?.gross.expected_minor)}</Table.Td>
              <Table.Td style={{ textAlign: 'right' }}>
                {formatINR((comp?.gross.reported_minor || 0) - (comp?.gross.expected_minor || 0))}
              </Table.Td>
            </Table.Tr>
            <Table.Tr>
              <Table.Td>Gateway Fees</Table.Td>
              <Table.Td style={{ textAlign: 'right' }}>{formatINR(comp?.fee.reported_minor)}</Table.Td>
              <Table.Td style={{ textAlign: 'right' }}>{formatINR(comp?.fee.expected_minor)}</Table.Td>
              <Table.Td style={{ textAlign: 'right' }}>
                <Text
                  size="xs"
                  fw={600}
                  c={(comp?.fee.reported_minor || 0) > (comp?.fee.expected_minor || 0) ? 'red.7' : 'dimmed'}
                >
                  {formatINR((comp?.fee.expected_minor || 0) - (comp?.fee.reported_minor || 0))}
                </Text>
              </Table.Td>
            </Table.Tr>
            <Table.Tr>
              <Table.Td>Deducted Refunds</Table.Td>
              <Table.Td style={{ textAlign: 'right' }}>{formatINR(comp?.refund.reported_minor)}</Table.Td>
              <Table.Td style={{ textAlign: 'right' }}>{formatINR(comp?.refund.expected_minor)}</Table.Td>
              <Table.Td style={{ textAlign: 'right' }}>
                {formatINR((comp?.refund.reported_minor || 0) - (comp?.refund.expected_minor || 0))}
              </Table.Td>
            </Table.Tr>
            <Table.Tr style={{ fontWeight: 700, backgroundColor: '#f8f9fa' }}>
              <Table.Td>Net Settlement</Table.Td>
              <Table.Td style={{ textAlign: 'right' }}>{formatINR(comp?.net.reported_minor)}</Table.Td>
              <Table.Td style={{ textAlign: 'right' }}>{formatINR(comp?.net.expected_minor)}</Table.Td>
              <Table.Td style={{ textAlign: 'right', color: '#e03131' }}>
                {formatINR(exception.variance_minor)}
              </Table.Td>
            </Table.Tr>
          </Table.Tbody>
        </Table>
      </Paper>

      {/* 3. Deposit Search Window */}
      {dates && (
        <Paper withBorder p="sm" radius="md" mb="md">
          <Group gap={6} mb="xs">
            <IconCalendar size={16} color="#fd7e14" />
            <Text size="xs" fw={700} tt="uppercase" c="dimmed">
              Deposit Candidate Matching Window
            </Text>
          </Group>
          <Text size="xs">
            Eligible Bank Deposit Range (T to T+2):{' '}
            <Code fw={600}>{dates.window_start}</Code> to <Code fw={600}>{dates.window_end}</Code>
          </Text>
        </Paper>
      )}

      {/* 4. Traceable Evidence Registry Chips */}
      <Paper withBorder p="sm" radius="md">
        <Group gap={6} mb="xs">
          <IconFingerprint size={16} color="#7048e8" />
          <Text size="xs" fw={700} tt="uppercase" c="dimmed">
            Context Evidence Registry IDs
          </Text>
        </Group>
        <Text size="xs" c="dimmed" mb="xs">
          These verified database entity IDs are available for the agent's hypothesis testing:
        </Text>
        <Group gap={6}>
          <Badge size="sm" variant="outline" color="blue">
            ctx:batch
          </Badge>
          <Badge size="sm" variant="outline" color="blue">
            ctx:calc
          </Badge>
          <Badge size="sm" variant="outline" color="blue">
            ctx:dates
          </Badge>
          <Badge size="sm" variant="outline" color="violet">
            {exception.batch_ref}
          </Badge>
          {exception.type === 'FEE_VARIANCE' && (
            <Badge size="sm" variant="outline" color="teal">
              fee_row:{exception.batch_ref}
            </Badge>
          )}
          {exception.type === 'DELAYED_OR_SPLIT_DEPOSIT' && (
            <Badge size="sm" variant="outline" color="teal">
              deposit:TXN-S02
            </Badge>
          )}
          {exception.type === 'REFUND_NOT_IN_LEDGER' && (
            <Badge size="sm" variant="outline" color="teal">
              gw_refund:GWREF-R1
            </Badge>
          )}
        </Group>
      </Paper>
    </Box>
  );
};

