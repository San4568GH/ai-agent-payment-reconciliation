import React from 'react';
import { SimpleGrid, Paper, Text, Group, ThemeIcon, Progress, Box } from '@mantine/core';
import {
  IconCreditCard,
  IconScale,
  IconBuildingBank,
  IconAlertTriangle,
  IconCircleCheck,
  IconLayersLinked,
} from '@tabler/icons-react';
import { Summary } from '../types';
import { formatINR } from '../utils/money';

interface SummaryCardsProps {
  summary: Summary | null;
  isLoading: boolean;
}

export const SummaryCards: React.FC<SummaryCardsProps> = ({ summary, isLoading }) => {
  const paymentVolume = summary ? formatINR(summary.payment_volume_minor) : '₹0.00';
  const expectedNet = summary ? formatINR(summary.expected_net_minor) : '₹0.00';
  const bankReceived = summary ? formatINR(summary.bank_received_minor) : '₹0.00';
  const netVariance = summary ? formatINR(summary.net_variance_minor) : '₹0.00';
  const hasVariance = summary ? summary.net_variance_minor !== 0 : false;

  const batchesMatched = summary?.batches_matched || 0;
  const batchesTotal = summary?.batches_total || 0;
  const matchPercentage = batchesTotal > 0 ? Math.round((batchesMatched / batchesTotal) * 100) : 0;
  const openExceptions = summary?.open_exceptions || 0;

  return (
    <SimpleGrid cols={{ base: 1, sm: 2, lg: 4 }} spacing="md" mb="lg">
      {/* 1. Payment Volume */}
      <Paper withBorder p="md" radius="md">
        <Group justify="space-between">
          <Text size="xs" c="dimmed" fw={700} tt="uppercase">
            Total Payment Volume
          </Text>
          <ThemeIcon color="blue" variant="light" size="md">
            <IconCreditCard size={18} />
          </ThemeIcon>
        </Group>
        <Group align="flex-end" gap="xs" mt="sm">
          <Text size="xl" fw={700}>
            {paymentVolume}
          </Text>
        </Group>
        <Text size="xs" c="dimmed" mt={4}>
          Gross processed across 6 settlement batches
        </Text>
      </Paper>

      {/* 2. Expected vs Bank Received */}
      <Paper withBorder p="md" radius="md">
        <Group justify="space-between">
          <Text size="xs" c="dimmed" fw={700} tt="uppercase">
            Bank Received / Expected
          </Text>
          <ThemeIcon color="teal" variant="light" size="md">
            <IconBuildingBank size={18} />
          </ThemeIcon>
        </Group>
        <Group align="flex-end" gap="xs" mt="sm">
          <Text size="xl" fw={700}>
            {bankReceived}
          </Text>
          <Text size="xs" c="dimmed" pb={3}>
            / {expectedNet}
          </Text>
        </Group>
        <Text size="xs" c="dimmed" mt={4}>
          Actual bank deposits vs net calculated settlement
        </Text>
      </Paper>

      {/* 3. Net Variance */}
      <Paper
        withBorder
        p="md"
        radius="md"
        style={{
          borderLeft: hasVariance ? '4px solid #fa5252' : '4px solid #40c057',
        }}
      >
        <Group justify="space-between">
          <Text size="xs" c="dimmed" fw={700} tt="uppercase">
            Net Variance (Bank − Expected)
          </Text>
          <ThemeIcon color={hasVariance ? 'red' : 'green'} variant="light" size="md">
            {hasVariance ? <IconAlertTriangle size={18} /> : <IconCircleCheck size={18} />}
          </ThemeIcon>
        </Group>
        <Group align="flex-end" gap="xs" mt="sm">
          <Text size="xl" fw={700} c={hasVariance ? 'red.7' : 'teal.7'}>
            {netVariance}
          </Text>
        </Group>
        <Text size="xs" c="dimmed" mt={4}>
          {hasVariance
            ? 'Total shortfall requiring agent investigation'
            : 'All batches balanced with 0 discrepancy'}
        </Text>
      </Paper>

      {/* 4. Batch Match Rate & Exceptions */}
      <Paper withBorder p="md" radius="md">
        <Group justify="space-between">
          <Text size="xs" c="dimmed" fw={700} tt="uppercase">
            Reconciliation Health
          </Text>
          <ThemeIcon color="indigo" variant="light" size="md">
            <IconLayersLinked size={18} />
          </ThemeIcon>
        </Group>
        <Group align="flex-end" gap="xs" mt="xs">
          <Text size="xl" fw={700}>
            {batchesMatched}/{batchesTotal}
          </Text>
          <Text size="xs" c="dimmed" pb={3}>
            Batches Matched ({matchPercentage}%)
          </Text>
        </Group>
        <Box mt={8}>
          <Progress value={matchPercentage} color={matchPercentage === 100 ? 'teal' : 'indigo'} size="sm" radius="xl" />
        </Box>
        <Text size="xs" c={openExceptions > 0 ? 'orange.7' : 'dimmed'} mt={6} fw={openExceptions > 0 ? 600 : 400}>
          {openExceptions} open exception{openExceptions !== 1 ? 's' : ''} detected
        </Text>
      </Paper>
    </SimpleGrid>
  );
};

