import React from 'react';
import { Paper, Title, Text, Button, ThemeIcon, Group, Box } from '@mantine/core';
import { IconDatabasePlus, IconSparkles } from '@tabler/icons-react';
import { UserRole } from '../types';

interface EmptyStateBannerProps {
  onLoadSeed: () => void;
  isLoading: boolean;
  currentRole: UserRole;
}

export const EmptyStateBanner: React.FC<EmptyStateBannerProps> = ({
  onLoadSeed,
  isLoading,
  currentRole,
}) => {
  return (
    <Paper
      withBorder
      p="xl"
      radius="md"
      mb="xl"
      style={{
        textAlign: 'center',
        backgroundColor: '#f8f9fa',
        border: '2px dashed #ced4da',
      }}
    >
      <Box style={{ maxWidth: 500, margin: '0 auto' }}>
        <ThemeIcon size={52} radius="xl" color="indigo" variant="light" mb="md">
          <IconDatabasePlus size={30} />
        </ThemeIcon>

        <Title order={3} mb="xs">
          Reconciliation Database is Empty
        </Title>

        <Text size="sm" c="dimmed" mb="lg">
          No gateway batches, bank deposits, or ledger records have been imported yet.
          Click below to load the deterministic seed files and run reconciliation.
        </Text>

        <Button
          size="md"
          color="indigo"
          leftSection={<IconSparkles size={18} />}
          loading={isLoading}
          disabled={currentRole === 'viewer'}
          onClick={onLoadSeed}
        >
          Load Deterministic Seed Data
        </Button>

        {currentRole === 'viewer' && (
          <Text size="xs" c="red" mt="xs">
            Switch to Operator role in the header to load seed data.
          </Text>
        )}
      </Box>
    </Paper>
  );
};

