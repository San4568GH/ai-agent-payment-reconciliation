import React from 'react';
import { Table, Badge, Paper, Text, Group } from '@mantine/core';
import { SettlementBatch } from '../types';
import { formatINR } from '../utils/money';

interface SettlementsTableProps {
  batches: SettlementBatch[];
}

export const SettlementsTable: React.FC<SettlementsTableProps> = ({ batches }) => {
  return (
    <Paper withBorder p="md" radius="md">
      <Group justify="space-between" mb="md">
        <div>
          <Text size="md" fw={700}>
            Gateway Settlement Batches
          </Text>
          <Text size="xs" c="dimmed">
            Official gateway settlement manifests compared against expected calculations and bank ledger postings
          </Text>
        </div>
      </Group>

      <Table.ScrollContainer minWidth={850}>
        <Table verticalSpacing="sm" highlightOnHover>
          <Table.Thead>
            <Table.Tr>
              <Table.Th>Batch Reference</Table.Th>
              <Table.Th>Settled On</Table.Th>
              <Table.Th>Reported Gross</Table.Th>
              <Table.Th>Reported Fee</Table.Th>
              <Table.Th>Reported Refund</Table.Th>
              <Table.Th>Reported Net</Table.Th>
              <Table.Th>Expected Net</Table.Th>
              <Table.Th>Bank Received</Table.Th>
              <Table.Th>Variance</Table.Th>
              <Table.Th>Status</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {batches.map((b) => {
              const isMatched = b.status === 'matched';
              const hasVariance = b.variance_minor !== 0;

              return (
                <Table.Tr key={b.id || b.batch_ref}>
                  <Table.Td>
                    <Text size="sm" fw={600} ff="monospace">
                      {b.batch_ref}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <Text size="xs">{b.settled_on}</Text>
                  </Table.Td>
                  <Table.Td>
                    <Text size="xs">{formatINR(b.reported_gross_minor)}</Text>
                  </Table.Td>
                  <Table.Td>
                    <Text size="xs" c="dimmed">
                      {formatINR(b.reported_fee_minor)}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <Text size="xs" c="dimmed">
                      {formatINR(b.reported_refund_minor)}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <Text size="xs" fw={600}>
                      {formatINR(b.reported_net_minor)}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <Text size="xs" fw={600} c="indigo.7">
                      {formatINR(b.expected_net_minor)}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <Text size="xs" fw={600} c="teal.7">
                      {formatINR(b.bank_received_minor)}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <Text size="xs" fw={700} c={hasVariance ? 'red.7' : 'teal.7'}>
                      {formatINR(b.variance_minor)}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <Badge color={isMatched ? 'teal' : 'red'} variant="light">
                      {isMatched ? 'MATCHED' : 'VARIANCE'}
                    </Badge>
                  </Table.Td>
                </Table.Tr>
              );
            })}
          </Table.Tbody>
        </Table>
      </Table.ScrollContainer>
    </Paper>
  );
};

