import { ref, computed, watch } from 'vue';
import { useVueFlow } from '@vue-flow/core';

/**
 * Checks if a file path is an image based on its extension
 * @param {string} filepath - The file path to check
 * @returns {boolean} True if the file appears to be an image
 */
function isImageFile(filepath) {
  if (!filepath) return false;
  const extension = filepath.toLowerCase().split('.').pop();
  const imageExtensions = ['jpg', 'jpeg', 'png', 'gif', 'bmp', 'webp', 'svg'];
  return imageExtensions.includes(extension);
}

/**
 * Converts binary data to base64
 * @param {ArrayBuffer} buffer - The binary data to convert
 * @returns {string} Base64 encoded data
 */
function arrayBufferToBase64(buffer) {
  let binary = '';
  const bytes = new Uint8Array(buffer);
  const len = bytes.byteLength;
  for (let i = 0; i < len; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  return window.btoa(binary);
}

/**
 * Get the correct content type based on file extension
 * @param {string} filepath - The file path to check
 * @returns {string} The content type
 */
function getContentType(filepath) {
  const extension = filepath.toLowerCase().split('.').pop();
  switch (extension) {
    case 'jpg':
    case 'jpeg':
      return 'image/jpeg';
    case 'png':
      return 'image/png';
    case 'gif':
      return 'image/gif';
    case 'svg':
      return 'image/svg+xml';
    case 'webp':
      return 'image/webp';
    case 'bmp':
      return 'image/bmp';
    default:
      return 'image/png'; // Default
  }
}

/**
 * Composable for managing OpenFileNode functionality
 * @param {Object} props - Component props
 * @param {Function} emit - Component emit function
 * @returns {Object} - OpenFileNode functionality
 */
export function useOpenFileNode(props, emit) {
  const { getEdges, findNode } = useVueFlow();

  // File path input
  const filepath = computed({
    get: () => props.data.inputs.filepath,
    set: (value) => {
      props.data.inputs.filepath = value;
      
      // Update isImage flag when filepath changes
      props.data.isImage = isImageFile(value);
    },
  });

  // Option to update from source
  const updateFromSource = ref(props.data.updateFromSource);
  
  // Track if the current file is an image
  const isImage = computed(() => isImageFile(filepath.value));

  /**
   * Updates the node data and emits changes
   */
  const updateNodeData = () => {
    // Make sure isImage flag is set based on current file path
    const isCurrentImage = isImageFile(filepath.value);
    
    const updatedData = {
      ...props.data,
      inputs: {
        filepath: filepath.value,
      },
      outputs: props.data.outputs,
      updateFromSource: updateFromSource.value,
      isImage: isCurrentImage,
    };
    
    emit('update:data', { id: props.id, data: updatedData });
  };

  /**
   * Loads an image from a path and returns base64 data and data URL
   * @param {string} imagePath - The path to the image
   * @returns {Promise<Object>} Object with base64 and dataUrl properties
   */
  async function loadImage(imagePath) {
    try {
      // For absolute paths or URLs, use fetch directly
      const response = await fetch(imagePath);
      
      if (!response.ok) {
        throw new Error(`Failed to load image: ${response.status} ${response.statusText}`);
      }
      
      const blob = await response.blob();
      const contentType = blob.type || getContentType(imagePath);
      
      return new Promise((resolve, reject) => {
        const reader = new FileReader();
        
        reader.onload = () => {
          const base64String = reader.result.split(',')[1];
          resolve({
            base64: base64String,
            dataUrl: reader.result,
          });
        };
        
        reader.onerror = () => {
          reject(new Error('Failed to read image file'));
        };
        
        reader.readAsDataURL(blob);
      });
    } catch (error) {
      console.error('Error loading image:', error);
      throw error;
    }
  }

  /**
   * Main run function that opens and reads a file
   * @returns {Promise<Object>} Result of the operation
   */
  async function run() {
    console.log('Running OpenFileNode:', props.id);

    // Identify connected source nodes
    const connectedSources = getEdges.value
      .filter((edge) => edge.target === props.id)
      .map((edge) => edge.source);

    let payload;

    // Handle connected sources if needed
    if (connectedSources.length > 0 && updateFromSource.value) {
      const sourceNode = findNode(connectedSources[0]);
      if (!sourceNode || !sourceNode.data || !sourceNode.data.outputs) {
        console.error('Connected source node data is invalid');
        props.data.outputs.result = {
          error: 'Invalid source node data',
        };
        return { error: 'Invalid source node data' };
      }

      // Safely get the source output data
      const sourceData = sourceNode.data.outputs.result?.output;
      console.log('Connected source data:', sourceData);

      if (!sourceData) {
        console.error('No valid output from connected node');
        props.data.outputs.result = {
          error: 'No valid output from connected node',
        };
        return { error: 'No valid output from connected node' };
      }

      // Update the input field with the connected source data
      filepath.value = sourceData;
      props.data.inputs.filepath = sourceData;

      // If the source data is JSON, try to parse it
      if (typeof sourceData === 'string' && sourceData.trim().startsWith('{')) {
        try {
          payload = JSON.parse(sourceData);
        } catch (err) {
          console.error('Error parsing JSON from connected node:', err);
          payload = { filepath: sourceData };
        }
      } else {
        payload = { filepath: sourceData };
      }
    } else {
      // No connected nodes or updateFromSource is false => use the input field value
      payload = { filepath: filepath.value };
    }

    try {
      // Check if we're dealing with an image file
      const currentFilePath = payload.filepath;
      const isImageRequest = isImageFile(currentFilePath);
      
      props.data.isImage = isImageRequest;
      
      if (isImageRequest) {
        console.log('Loading image from:', currentFilePath);
        
        try {
          // load original image
          const { base64: origBase64, dataUrl: origDataUrl } = await loadImage(currentFilePath);
          const img = new Image();
          await new Promise((resolve, reject) => {
            img.onload = resolve;
            img.onerror = reject;
            img.src = origDataUrl;
          });

          // resize to 256×256 (stretch to fit)
          const TARGET_WIDTH = 256, TARGET_HEIGHT = 256;
          let resizedDataUrl = origDataUrl;
          let resizedBase64 = origBase64;
          if (img.width !== TARGET_WIDTH || img.height !== TARGET_HEIGHT) {
            console.log(`Resizing image from ${img.width}×${img.height} to ${TARGET_WIDTH}×${TARGET_HEIGHT}`);
            const canvas = document.createElement('canvas');
            canvas.width = TARGET_WIDTH;
            canvas.height = TARGET_HEIGHT;
            const ctx = canvas.getContext('2d');
            ctx.drawImage(img, 0, 0, TARGET_WIDTH, TARGET_HEIGHT);
            resizedDataUrl = canvas.toDataURL(getContentType(currentFilePath));
            resizedBase64 = resizedDataUrl.split(',')[1];
          }
          // original image for display, processedDataUrl for processing
          props.data.outputs = {
            result: {
              output: resizedBase64,
              dataUrl: origDataUrl,
              processedDataUrl: resizedDataUrl
            }
          };
        } catch (imageError) {
          console.error('Failed to load image:', imageError);
          // Fall back to the API if direct loading fails
          console.log('Falling back to API for image loading');
          
          const response = await fetch('http://localhost:8080/api/open-file', {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
            },
            body: JSON.stringify({
              filepath: currentFilePath,
            }),
          });
          
          if (!response.ok) {
            throw new Error(`API error: ${response.status}`);
          }
          
          // Get binary data from API response
          const buffer = await response.arrayBuffer();
          const base64Data = arrayBufferToBase64(buffer);
          const contentType = getContentType(currentFilePath);
          const dataUrl = `data:${contentType};base64,${base64Data}`;
          
          // Set both raw base64 and data URL 
          props.data.outputs = {
            result: {
              output: base64Data,
              dataUrl: dataUrl,
            },
          };
        }
      } else {
        // Standard text file handling
        const response = await fetch('http://localhost:8080/api/open-file', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            filepath: payload.filepath,
          }),
        });

        if (!response.ok) {
          const errorData = await response.json();
          console.error('Error reading file content:', errorData.error);
          props.data.outputs.result = {
            error: errorData.error,
          };
          return { error: errorData.error };
        } else {
          const fileContent = await response.text();
          console.log('File content:', fileContent.substring(0, 100) + (fileContent.length > 100 ? '...' : ''));
          props.data.outputs = {
            result: {
              output: fileContent,
            },
          };
        }
      }
    } catch (error) {
      console.error('Error opening file:', error);
      props.data.outputs.result = {
        error: error.message,
      };
      return { error: error.message };
    }

    updateNodeData(); // Update data after processing
    return { result: props.data.outputs.result };
  }

  // Watch for filepath changes
  watch(filepath, () => {
    updateNodeData();
  });

  return {
    filepath,
    updateFromSource,
    updateNodeData,
    run,
    isImage,
  };
}