import React from 'react';
import {
  Accordion,
  Paper,
  Text,
  Badge,
  Group,
  Box,
  Code,
  Loader,
  Alert,
  ThemeIcon,
  Timeline,
} from '@mantine/core';
import {
  IconTool,
  IconCheck,
  IconAlertTriangle,
  IconClock,
  IconSparkles,
  IconRobot,
  IconArrowRight,
  IconFileCertificate,
} from '@tabler/icons-react';
import { WorkflowDetail, Remediation, UserRole } from '../types';
import { RemediationPanel } from './RemediationPanel';

interface AgentTraceProps {
  workflowDetail: WorkflowDetail | null;
  isLoading: boolean;
  remediation?: Remediation;
  onApproveRemediation: (id: number) => void;
  isApproving: boolean;
  onRejectRemediation: (id: number, note?: string) => void;
  isRejecting: boolean;
  onRetryRemediation: (id: number) => void;
  isRetrying: boolean;
  currentRole: UserRole;
}

export const AgentTrace: React.FC<AgentTraceProps> = ({
  workflowDetail,
  isLoading,
  remediation,
  onApproveRemediation,
  isApproving,
  onRejectRemediation,
  isRejecting,
  onRetryRemediation,
  isRetrying,
  currentRole,
}) => {
  if (isLoading && !workflowDetail) {
    return (
      <Paper withBorder p="xl" radius="md" style={{ textAlign: 'center' }}>
        <Loader size="md" color="indigo" mb="sm" />
        <Text size="sm" fw={600}>
          Loading agent investigation steps...
        </Text>
      </Paper>
    );
  }

  if (!workflowDetail) {
    return (
      <Paper withBorder p="xl" radius="md" style={{ textAlign: 'center' }}>
        <ThemeIcon size={40} radius="xl" color="gray" variant="light" mb="sm">
          <IconRobot size={24} />
        </ThemeIcon>
        <Text size="sm" fw={600} c="dimmed">
          No investigation run recorded yet.
        </Text>
        <Text size="xs" c="dimmed" mt={4}>
          Click "Investigate" to trigger autonomous hypothesis testing and bounded evidence gathering.
        </Text>
      </Paper>
    );
  }

  const { workflow, steps } = workflowDetail;
  const isRunning = workflow.status === 'running';

  return (
    <Box>
      {/* Run Status Banner */}
      <Paper withBorder p="sm" radius="md" mb="md" style={{ backgroundColor: '#ffffff' }}>
        <Group justify="space-between" align="center">
          <Group gap="xs">
            <IconSparkles size={18} color="#4c6ef5" />
            <Text size="sm" fw={700}>
              Investigation Run #{workflow.id} (Attempt {workflow.attempt})
            </Text>
          </Group>
          <Badge
            color={
              workflow.status === 'completed'
                ? 'teal'
                : workflow.status === 'running'
                ? 'yellow'
                : workflow.status === 'escalated'
                ? 'violet'
                : 'red'
            }
            size="md"
          >
            {workflow.status.toUpperCase()}
          </Badge>
        </Group>

        <Group gap="md" mt="xs">
          <Text size="xs">
            <strong>Tool Calls:</strong> {workflow.tool_calls}/6 max
          </Text>
          <Text size="xs">
            <strong>LLM Turns:</strong> {workflow.llm_turns}/8 max
          </Text>
          {workflow.started_at && (
            <Text size="xs" c="dimmed">
              Started: {new Date(workflow.started_at).toLocaleTimeString()}
            </Text>
          )}
        </Group>

        {isRunning && (
          <Group gap="xs" mt="sm">
            <Loader size="xs" color="indigo" />
            <Text size="xs" c="indigo.7" fw={600}>
              Autonomous investigation loop active (testing hypotheses & inspecting evidence)...
            </Text>
          </Group>
        )}
      </Paper>

      {/* Accordion of Step-by-Step Tool Calls */}
      <Text size="xs" fw={700} tt="uppercase" c="dimmed" mb="xs">
        Execution Trace ({steps.length} Step{steps.length !== 1 ? 's' : ''})
      </Text>

      <Accordion variant="separated" radius="md" defaultValue={`step-${steps.length}`}>
        {steps.map((step) => {
          const isError = step.status === 'error';

          return (
            <Accordion.Item key={step.id || step.seq} value={`step-${step.seq}`}>
              <Accordion.Control>
                <Group justify="space-between">
                  <Group gap="xs">
                    <ThemeIcon color={isError ? 'red' : 'indigo'} size="sm" variant="light">
                      <IconTool size={14} />
                    </ThemeIcon>
                    <Text size="xs" fw={700}>
                      Step {step.seq}: <Code>{step.tool_name}</Code>
                    </Text>
                  </Group>
                  <Group gap="xs">
                    {step.duration_ms !== undefined && (
                      <Badge size="xs" variant="outline" color="gray" leftSection={<IconClock size={10} />}>
                        {step.duration_ms}ms
                      </Badge>
                    )}
                    <Badge size="xs" color={isError ? 'red' : 'teal'}>
                      {step.status}
                    </Badge>
                  </Group>
                </Group>
              </Accordion.Control>
              <Accordion.Panel>
                {/* Arguments */}
                <Box mb="xs">
                  <Text size="xs" c="dimmed" fw={600}>
                    Tool Input Arguments:
                  </Text>
                  <Code block style={{ fontSize: 11 }}>
                    {JSON.stringify(step.args, null, 2)}
                  </Code>
                </Box>

                {/* Output Result */}
                <Box mb="xs">
                  <Text size="xs" c="dimmed" fw={600}>
                    Sanitized Tool Output (Untrusted Data):
                  </Text>
                  <Code block style={{ fontSize: 11, maxHeight: 180, overflowY: 'auto' }}>
                    {JSON.stringify(step.result, null, 2)}
                  </Code>
                </Box>

                {/* Cited Evidence IDs */}
                {step.evidence_ids && step.evidence_ids.length > 0 && (
                  <Box mt="xs">
                    <Text size="xs" c="dimmed" fw={600} mb={4}>
                      Evidence IDs Registered:
                    </Text>
                    <Group gap={4}>
                      {step.evidence_ids.map((eid) => (
                        <Badge key={eid} size="xs" variant="dot" color="teal">
                          {eid}
                        </Badge>
                      ))}
                    </Group>
                  </Box>
                )}
              </Accordion.Panel>
            </Accordion.Item>
          );
        })}
      </Accordion>

      {/* Terminal Agent Conclusion (Note 8: labeled "Agent summary") */}
      {workflow.conclusion && (
        <Paper
          withBorder
          p="md"
          radius="md"
          mt="md"
          style={{
            backgroundColor: '#f8f9fa',
            borderLeft: '4px solid #1c7ed6',
          }}
        >
          <Group gap="xs" mb="xs">
            <IconFileCertificate size={18} color="#1c7ed6" />
            <Text size="sm" fw={700}>
              Agent summary
            </Text>
            <Badge size="sm" color="blue" variant="light">
              {workflow.conclusion.conclusion}
            </Badge>
          </Group>

          <Text size="xs" style={{ whiteSpace: 'pre-wrap' }}>
            {workflow.conclusion.summary}
          </Text>

          {workflow.conclusion.cited_evidence_ids && workflow.conclusion.cited_evidence_ids.length > 0 && (
            <Box mt="xs">
              <Text size="xs" c="dimmed" fw={600} mb={4}>
                Authoritative Cited Evidence:
              </Text>
              <Group gap={4}>
                {workflow.conclusion.cited_evidence_ids.map((id) => (
                  <Badge key={id} size="xs" variant="outline" color="indigo">
                    {id}
                  </Badge>
                ))}
              </Group>
            </Box>
          )}
        </Paper>
      )}

      {/* Human-In-The-Loop Remediation Card */}
      {remediation && (
        <RemediationPanel
          remediation={remediation}
          onApprove={onApproveRemediation}
          isApproving={isApproving}
          onReject={onRejectRemediation}
          isRejecting={isRejecting}
          onRetry={onRetryRemediation}
          isRetrying={isRetrying}
          currentRole={currentRole}
        />
      )}
    </Box>
  );
};

