<template>
    <div id="cb-header" class="header-container" hx-preserve>
        <div id="hgradient"></div>
        <div class="box-container">
            <!-- Manifold Title Logo -->
            Manifold 
        </div>
    </div>
</template>

<style scoped>
#cb-header {
    position: relative;
    height: 50px;
}

.box-container {
    position: absolute;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    /* Roboto Font */
    font-family: 'Roboto', sans-serif;
    font-size: 24px;
    font-weight: 700;
    color: white;
}
</style>