export function isNodeConnected(nodeId, edges) {
    return edges.some(edge => edge.source === nodeId || edge.target === nodeId);
}

export function isNodeSelected(nodeId, selectedNodes) {
    return selectedNodes.includes(nodeId);
}

export function isNodeHovered(nodeId, hoveredNode) {
    return nodeId === hoveredNode;
}

export function isNodeDisabled(nodeId, disabledNodes) {
    return disabledNodes.includes(nodeId);
}