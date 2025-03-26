import { ref, onMounted, computed, nextTick } from 'vue'

/**
 * Composable for managing GLSLNode state and functionality
 */
export function useGLSLNode(props, emit, vueFlow) {
  const { getEdges, findNode } = vueFlow

  // State variables
  const isHovered = ref(false)
  const shaderCanvas = ref(null)
  const canvasContext = ref(null)
  const programInfo = ref(null)
  const buffers = ref(null)
  const lastFrameTime = ref(0)
  const isAnimating = ref(false)
  const animationId = ref(null)
  const customStyle = ref({})
  
  // UI state
  const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? 'visible' : 'hidden',
    width: '12px',
    height: '12px'
  }))
  
  // Computed properties for form binding
  const fragmentShader = computed({
    get: () => props.data.inputs.fragmentShader,
    set: (value) => { props.data.inputs.fragmentShader = value }
  })

  // Start animation loop
  function startAnimating() {
    if (!isAnimating.value) {
      isAnimating.value = true
      animationId.value = requestAnimationFrame(render)
    }
  }

  // Stop animation loop
  function stopAnimating() {
    if (isAnimating.value && animationId.value !== null) {
      cancelAnimationFrame(animationId.value)
      isAnimating.value = false
      animationId.value = null
    }
  }
  
  // Handle resize of the node
  function onResize(event) {
    customStyle.value.width = `${event.width}px`
    customStyle.value.height = `${event.height}px`
    if (emit) {
      emit('resize', event)
    }
    
    // We need to manually resize the canvas and recompile the shader
    nextTick(() => {
      if (shaderCanvas.value) {
        resizeCanvas()
        run()
      }
    })
  }
  
  // Resize canvas to fit container
  function resizeCanvas() {
    if (!shaderCanvas.value) return
    
    const container = shaderCanvas.value.parentElement
    if (!container) return
    
    shaderCanvas.value.width = container.clientWidth
    shaderCanvas.value.height = container.clientHeight
    
    if (canvasContext.value) {
      canvasContext.value.viewport(0, 0, shaderCanvas.value.width, shaderCanvas.value.height)
    }
  }

  // Compile shader program
  function initShaderProgram(gl, vsSource, fsSource) {
    const vertexShader = loadShader(gl, gl.VERTEX_SHADER, vsSource)
    const fragmentShader = loadShader(gl, gl.FRAGMENT_SHADER, fsSource)

    if (!vertexShader || !fragmentShader) {
      return null
    }

    // Create the shader program
    const shaderProgram = gl.createProgram()
    gl.attachShader(shaderProgram, vertexShader)
    gl.attachShader(shaderProgram, fragmentShader)
    gl.linkProgram(shaderProgram)

    // If creating the shader program failed, alert
    if (!gl.getProgramParameter(shaderProgram, gl.LINK_STATUS)) {
      console.error('Unable to initialize shader program: ' + gl.getProgramInfoLog(shaderProgram))
      return null
    }

    return shaderProgram
  }

  // Load and compile shader
  function loadShader(gl, type, source) {
    const shader = gl.createShader(type)
    gl.shaderSource(shader, source)
    gl.compileShader(shader)

    // Check if shader compiled successfully
    if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
      console.error('Shader compilation error: ' + gl.getShaderInfoLog(shader))
      gl.deleteShader(shader)
      return null
    }

    return shader
  }

  // Initialize buffers for quad
  function initBuffers(gl) {
    const positions = [
      -1.0, -1.0,
       1.0, -1.0,
      -1.0,  1.0,
       1.0,  1.0,
    ]

    const positionBuffer = gl.createBuffer()
    gl.bindBuffer(gl.ARRAY_BUFFER, positionBuffer)
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array(positions), gl.STATIC_DRAW)

    return {
      position: positionBuffer,
    }
  }

  // Draw the scene
  function render(now) {
    if (!isAnimating.value) return
    
    now *= 0.001  // Convert to seconds
    const deltaTime = now - lastFrameTime.value
    lastFrameTime.value = now

    const gl = canvasContext.value
    if (!gl || !programInfo.value || !buffers.value) return

    gl.clearColor(0.0, 0.0, 0.0, 1.0)
    gl.clear(gl.COLOR_BUFFER_BIT)

    gl.useProgram(programInfo.value.program)

    // Set up time uniform
    gl.uniform1f(programInfo.value.uniformLocations.uTime, now)
    
    // Set up resolution uniform
    gl.uniform2f(
      programInfo.value.uniformLocations.uResolution, 
      gl.canvas.width, 
      gl.canvas.height
    )
    
    // Set up vertex position attribute
    gl.bindBuffer(gl.ARRAY_BUFFER, buffers.value.position)
    gl.vertexAttribPointer(
      programInfo.value.attribLocations.vertexPosition,
      2,          // 2 components per vertex
      gl.FLOAT,   // Type
      false,      // Don't normalize
      0,          // Stride (0 = automatic)
      0           // Offset
    )
    gl.enableVertexAttribArray(programInfo.value.attribLocations.vertexPosition)

    // Draw the quad
    gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4)

    // Request next frame
    animationId.value = requestAnimationFrame(render)
  }

  // Initialize the shader program
  function setupShader() {
    if (!shaderCanvas.value) return
    
    const gl = shaderCanvas.value.getContext('webgl') || shaderCanvas.value.getContext('experimental-webgl')
    
    if (!gl) {
      console.error('WebGL not supported')
      return
    }
    
    // Store the context
    canvasContext.value = gl
    
    // Vertex shader program - just a passthrough for fragment shader
    const vsSource = `
      attribute vec4 aVertexPosition;
      varying vec2 vTextureCoord;
      void main() {
        gl_Position = aVertexPosition;
        // Map from [-1, 1] to [0, 1]
        vTextureCoord = aVertexPosition.xy * 0.5 + 0.5;
      }
    `
    
    // Use provided fragment shader or a default one
    const fsSource = fragmentShader.value || `
      precision mediump float;
      varying vec2 vTextureCoord;
      uniform float uTime;
      uniform vec2 uResolution;
      
      void main() {
        vec2 uv = vTextureCoord;
        vec3 col = 0.5 + 0.5 * cos(uTime + uv.xyx + vec3(0, 2, 4));
        gl_FragColor = vec4(col, 1.0);
      }
    `
    
    // Initialize shader program
    const shaderProgram = initShaderProgram(gl, vsSource, fsSource)
    if (!shaderProgram) {
      console.error('Could not initialize shader program')
      return
    }
    
    // Store program info
    programInfo.value = {
      program: shaderProgram,
      attribLocations: {
        vertexPosition: gl.getAttribLocation(shaderProgram, 'aVertexPosition'),
      },
      uniformLocations: {
        uTime: gl.getUniformLocation(shaderProgram, 'uTime'),
        uResolution: gl.getUniformLocation(shaderProgram, 'uResolution'),
      },
    }
    
    // Initialize buffers for quad
    buffers.value = initBuffers(gl)
    
    // Start animation loop
    startAnimating()
  }
  
  // Main run function
  async function run() {
    console.log('Running GLSLNode:', props.id)
    
    // Check for connected source nodes
    const connectedSources = getEdges.value
      .filter((edge) => edge.target === props.id)
      .map((edge) => edge.source)
    
    // Update shader from connected node if available
    if (connectedSources.length > 0) {
      const sourceNode = findNode(connectedSources[0])
      
      if (sourceNode && sourceNode.data.outputs.result) {
        fragmentShader.value = sourceNode.data.outputs.result.output
      }
    }
    
    // (Re)initialize the shader
    stopAnimating()
    await nextTick()
    setupShader()
  }
  
  // Lifecycle hooks
  onMounted(() => {
    if (!props.data.run) {
      props.data.run = run
    }
    
    // Initialize on mount
    nextTick(() => {
      resizeCanvas()
      setupShader()
    })
  })
  
  return {
    // State refs
    isHovered,
    shaderCanvas,
    fragmentShader,
    customStyle,
    
    // Computed
    resizeHandleStyle,
    
    // Methods
    onResize,
    run
  }
}