<template>
  <svg xmlns="http://www.w3.org/2000/svg" width="1em" height="1em" viewBox="0 0 24 24">
    <defs>
      <mask id="chatSparkle">
        <!-- bubble = visible (white in mask) -->
        <path fill="#fff" d="M12 2C6.477 2 2 5.94 2 10.8c0 2.78 1.46 5.255 3.74 6.87c-.16 1.04-.62 2.18-1.46 3.2c-.26.31-.03.79.37.74c1.99-.27 3.54-1.02 4.66-1.76c.86.2 1.77.31 2.69.31c5.523 0 10-3.94 10-8.8S17.523 2 12 2" />
        <!-- AI sparkle = cut out (black in mask) -->
        <path fill="#000" d="m12 6l.84 2.27c.16.43.24.65.37.83q.18.24.42.42c.18.13.4.21.83.37L17 11l-2.27.84c-.43.16-.65.24-.83.37q-.24.18-.42.42c-.13.18-.21.4-.37.83L12 16l-.84-2.27c-.16-.43-.24-.65-.37-.83a2 2 0 0 0-.42-.42c-.18-.13-.4-.21-.83-.37L7 11l2.27-.84c.43-.16.65-.24.83-.37q.24-.18.42-.42c.13-.18.21-.4.37-.83z" />
      </mask>
    </defs>
    <!-- single solid fill, same color as the rest of the icon set -->
    <rect width="24" height="24" fill="currentColor" mask="url(#chatSparkle)" />
  </svg>
</template>
