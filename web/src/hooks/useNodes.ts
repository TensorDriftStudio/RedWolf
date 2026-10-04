import { useState, useEffect, useCallback } from 'react';
import type { ServerNode, DeploymentConfig } from '../types';
import { getAuthHeaders } from '../utils/auth';

export function useNodes() {
  const [nodes, setNodes] = useState<ServerNode[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const [isConnected, setIsConnected] = useState<boolean>(false);

  // Fetch all nodes from RedWolf Core REST API
  const fetchNodes = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await fetch('/api/nodes', {
        headers: { ...getAuthHeaders() },
      });
      if (res.ok) {
        const data = await res.json();
        if (Array.isArray(data)) {
          setNodes(data);
        } else {
          setNodes([]);
        }
      } else {
        const errData = await res.json().catch(() => ({}));
        setError(errData.error || `Failed fetching fleet inventory (${res.status})`);
      }
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : 'Unable to connect to RedWolf Core API';
      setError(message);
    } finally {
      setIsLoading(false);
    }
  }, []);

  // Deploy bare-metal OS on a node
  const deployNode = useCallback(async (config: DeploymentConfig) => {
    try {
      const res = await fetch(`/api/nodes/${config.nodeId}/deploy`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...getAuthHeaders(),
        },
        body: JSON.stringify(config),
      });

      if (!res.ok) {
        const errData = await res.json().catch(() => ({ error: 'Deployment request rejected' }));
        throw new Error(errData.error || 'Deployment failed');
      }

      // Optimistically update local status to PROVISIONING
      setNodes((prev) =>
        prev.map((n) => {
          if (n.id === config.nodeId) {
            return {
              ...n,
              status: 'PROVISIONING',
              provisioningState: {
                progress: 5,
                stage: `Initiating ${config.os} deployment...`,
                os: config.os,
                targetDrive: config.targetDrivePath,
                targetIp: config.networkMode === 'static' ? config.staticIp || 'Static' : 'DHCP',
                logs: [`Deployment session dispatched to RedWolf Core`],
              },
            };
          }
          return n;
        })
      );
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : 'Unknown deployment error';
      setError(message);
      throw err;
    }
  }, []);

  // Reset an active or error node back to READY_FOR_PROVISIONING
  const resetNode = useCallback(async (nodeId: string) => {
    try {
      const res = await fetch(`/api/nodes/${nodeId}/reset`, {
        method: 'POST',
        headers: { ...getAuthHeaders() },
      });
      if (!res.ok) {
        const errData = await res.json().catch(() => ({ error: 'Failed resetting node' }));
        throw new Error(errData.error || 'Reset failed');
      }
      const updatedNode = await res.json();
      setNodes((prev) => prev.map((n) => (n.id === nodeId ? updatedNode : n)));
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : 'Unknown reset error';
      setError(message);
      throw err;
    }
  }, []);

  // Delete/decommission a node from RedWolf Core
  const deleteNode = useCallback(async (nodeId: string) => {
    try {
      const res = await fetch(`/api/nodes/${nodeId}`, {
        method: 'DELETE',
        headers: { ...getAuthHeaders() },
      });
      if (!res.ok) {
        const errData = await res.json().catch(() => ({ error: 'Failed deleting node' }));
        throw new Error(errData.error || 'Delete failed');
      }
      setNodes((prev) => prev.filter((n) => n.id !== nodeId));
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : 'Unknown delete error';
      setError(message);
      throw err;
    }
  }, []);

  // Real-time WebSocket connection to RedWolf Core event bus
  useEffect(() => {
    fetchNodes();

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws/events`;
    let ws: WebSocket | null = null;
    let reconnectTimeout: number | undefined;

    const connect = () => {
      try {
        ws = new WebSocket(wsUrl);

        ws.onopen = () => {
          setIsConnected(true);
        };

        ws.onmessage = (event) => {
          try {
            const data = JSON.parse(event.data);
            if (data.event === 'node:updated' || data.event === 'node:discovered') {
              const updatedNode = data.payload as ServerNode;
              setNodes((prev) => {
                const exists = prev.some((n) => n.id === updatedNode.id);
                if (exists) {
                  return prev.map((n) => (n.id === updatedNode.id ? updatedNode : n));
                }
                return [updatedNode, ...prev];
              });
            } else if (data.event === 'node:deleted') {
              const deletedId = data.payload?.id;
              if (deletedId) {
                setNodes((prev) => prev.filter((n) => n.id !== deletedId));
              }
            }
          } catch {
            // Ignore malformed payloads
          }
        };

        ws.onclose = () => {
          setIsConnected(false);
          reconnectTimeout = window.setTimeout(connect, 3000);
        };

        ws.onerror = () => {
          setIsConnected(false);
          ws?.close();
        };
      } catch {
        setIsConnected(false);
      }
    };

    connect();

    return () => {
      clearTimeout(reconnectTimeout);
      if (ws) {
        ws.onclose = null;
        ws.close();
      }
    };
  }, [fetchNodes]);

  return {
    nodes,
    setNodes,
    isLoading,
    error,
    isConnected,
    refresh: fetchNodes,
    deployNode,
    resetNode,
    deleteNode,
  };
}
