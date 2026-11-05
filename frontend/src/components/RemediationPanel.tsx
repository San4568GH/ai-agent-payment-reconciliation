import React, { useState } from 'react';
import {
  Paper,
  Text,
  Badge,
  Group,
  Button,
  Box,
  Divider,
  Alert,
  Code,
  Tooltip,
  TextInput,
} from '@mantine/core';
import {
  IconCheck,
  IconX,
  IconRefresh,
  IconTicket,
  IconLock,
  IconShieldCheck,
  IconAlertCircle,
} from '@tabler/icons-react';
import { Remediation, UserRole } from '../types';
import { formatINR } from '../utils/money';

interface RemediationPanelProps {
  remediation: Remediation;
  onApprove: (id: number) => void;
  isApproving: boolean;
  onReject: (id: number, note?: string) => void;
  isRejecting: boolean;
  onRetry: (id: number) => void;
  isRetrying: boolean;
  currentRole: UserRole;
}

export const RemediationPanel: React.FC<RemediationPanelProps> = ({
  remediation,
  onApprove,
  isApproving,
  onReject,
  isRejecting,
  onRetry,
  isRetrying,
  currentRole,
}) => {
  const [rejectNote, setRejectNote] = useState('');
  const [showRejectInput, setShowRejectInput] = useState(false);

  const isProposed = remediation.status === 'proposed';
  const isApproved = remediation.status === 'approved';
  const isExecuting = remediation.status === 'executing';
  const isExecuted = remediation.status === 'executed';
  const isRejected = remediation.status === 'rejected';
  const isFailed = remediation.status === 'execution_failed';

  const isViewer = currentRole === 'viewer';

  const getStatusColor = () => {
    switch (remediation.status) {
      case 'executed':
        return 'teal';
      case 'proposed':
        return 'indigo';
      case 'execution_failed':
        return 'red';
      case 'rejected':
        return 'gray';
      case 'executing':
        return 'yellow';
      default:
        return 'blue';
    }
  };

  return (
    <Paper
      withBorder
      p="md"
      radius="md"
      mt="md"
      style={{
        borderTop: `4px solid ${
          isExecuted ? '#40c057' : isFailed ? '#fa5252' : '#4c6ef5'
        }`,
        backgroundColor: '#ffffff',
      }}
    >
      <Group justify="space-between" align="center" mb="xs">
        <Group gap="xs">
          <IconShieldCheck size={18} color="#4c6ef5" />
          <Text size="sm" fw={700}>
            Proposed Corrective Action (HITL)
          </Text>
        </Group>
        <Badge color={getStatusColor()} size="md" variant="filled">
          {remediation.status.toUpperCase()}
        </Badge>
      </Group>

      {/* Action Type & Rationale */}
      <Box mb="xs">
        <Text size="xs" c="dimmed">
          Action Policy:
        </Text>
        <Badge variant="light" color="indigo" size="lg" mt={2}>
          {remediation.action_type}
        </Badge>
      </Box>

      <Box mb="xs">
        <Text size="xs" c="dimmed">
          Agent Technical Rationale:
        </Text>
        <Paper withBorder p="xs" radius="sm" style={{ backgroundColor: '#f8f9fa' }} mt={4}>
          <Text size="xs" style={{ whiteSpace: 'pre-wrap' }}>
            {remediation.rationale}
          </Text>
        </Paper>
      </Box>

      {/* Derived Payload Parameters */}
      {remediation.payload && Object.keys(remediation.payload).length > 0 && (
        <Box mb="xs">
          <Text size="xs" c="dimmed" mb={4}>
            Go-Calculated Financial Parameters:
          </Text>
          <Code block style={{ fontSize: 11 }}>
            {JSON.stringify(remediation.payload, null, 2)}
          </Code>
        </Box>
      )}

      {/* Idempotency Key Badge */}
      <Box mb="md">
        <Text size="xs" c="dimmed">
          SHA-256 Idempotency Key:
        </Text>
        <Code style={{ fontSize: 10, wordBreak: 'break-all' }}>
          {remediation.idempotency_key}
        </Code>
      </Box>

      {/* Executed Success Card */}
      {isExecuted && (
        <Alert
          icon={<IconTicket size={16} />}
          title="Executed against External System"
          color="teal"
          variant="light"
          mb="sm"
        >
          <Text size="xs">
            Remediation executed successfully and recorded in audit ledger.
          </Text>
          {remediation.external_ref && (
            <Text size="xs" fw={700} mt={4}>
              External System Ticket: <Code>{remediation.external_ref}</Code>
            </Text>
          )}
          {remediation.executed_at && (
            <Text size="xs" c="dimmed" mt={2}>
              Timestamp: {new Date(remediation.executed_at).toLocaleString()}
            </Text>
          )}
        </Alert>
      )}

      {/* Execution Failed Card */}
      {isFailed && (
        <Alert
          icon={<IconAlertCircle size={16} />}
          title="Execution Failed (Simulated or Downstream Error)"
          color="red"
          variant="light"
          mb="sm"
        >
          <Text size="xs">
            {remediation.last_error || 'Downstream external API returned an error.'}
          </Text>
          <Text size="xs" c="dimmed" mt={4}>
            Retry is safe and idempotent using the existing SHA-256 key.
          </Text>
        </Alert>
      )}

      {/* Rejection Card */}
      {isRejected && (
        <Alert icon={<IconX size={16} />} title="Rejected by Operator" color="gray" variant="light" mb="sm">
          <Text size="xs">This remediation proposal was rejected by operator.</Text>
          {remediation.decision_note && (
            <Text size="xs" mt={2}>
              Note: {remediation.decision_note}
            </Text>
          )}
        </Alert>
      )}

      {/* Action Buttons */}
      <Divider my="sm" />

      {isProposed && (
        <Box>
          {showRejectInput ? (
            <Box mb="xs">
              <TextInput
                size="xs"
                placeholder="Reason for rejecting this remediation..."
                value={rejectNote}
                onChange={(e) => setRejectNote(e.currentTarget.value)}
                mb="xs"
              />
              <Group gap="xs">
                <Button
                  size="xs"
                  color="red"
                  loading={isRejecting}
                  onClick={() => onReject(remediation.id, rejectNote)}
                >
                  Confirm Rejection
                </Button>
                <Button size="xs" variant="subtle" color="gray" onClick={() => setShowRejectInput(false)}>
                  Cancel
                </Button>
              </Group>
            </Box>
          ) : (
            <Group justify="space-between">
              <Tooltip
                label={
                  isViewer
                    ? 'Viewer role is read-only (Operator token required)'
                    : 'Approve & synchronously execute this remediation'
                }
              >
                <Button
                  size="sm"
                  color="teal"
                  leftSection={<IconCheck size={16} />}
                  loading={isApproving}
                  disabled={isViewer}
                  onClick={() => onApprove(remediation.id)}
                >
                  Approve & Execute
                </Button>
              </Tooltip>

              <Tooltip
                label={
                  isViewer
                    ? 'Viewer role is read-only'
                    : 'Reject this remediation proposal'
                }
              >
                <Button
                  size="sm"
                  variant="outline"
                  color="red"
                  leftSection={<IconX size={16} />}
                  disabled={isViewer}
                  onClick={() => setShowRejectInput(true)}
                >
                  Reject Proposal
                </Button>
              </Tooltip>
            </Group>
          )}
        </Box>
      )}

      {/* Retry Button if Failed */}
      {isFailed && (
        <Group justify="flex-end">
          <Tooltip
            label={
              isViewer
                ? 'Viewer role is read-only'
                : 'Retry execution against mock service'
            }
          >
            <Button
              size="sm"
              color="orange"
              leftSection={<IconRefresh size={16} />}
              loading={isRetrying}
              disabled={isViewer}
              onClick={() => onRetry(remediation.id)}
            >
              Retry Execution
            </Button>
          </Tooltip>
        </Group>
      )}
    </Paper>
  );
};

