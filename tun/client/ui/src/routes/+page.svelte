<script>
  import { onMount } from "svelte";

  // Tunnels, ACME data, domain input
  let tunnels = [];
  let domain = "";
  let acmeOutput = null;

  // Alert message system
  let message = "";
  let alertType = "error"; // "error" or "success"

  function closeAlert() {
    message = "";
  }

  // Confirmation modal state
  let showConfirmModal = false;
  let confirmMessage = "";
  let actionToConfirm = null;

  // Current tab
  let activeTab = "tunnels"; // default tab

  // Current config text
  let configText = "";

  /**
   * Load all tunnels, optionally in "quiet" mode.
   * If quiet === false, we show a success message; otherwise skip it.
   */
  async function loadTunnels(quiet = false) {
    try {
      const res = await fetch("/ls");
      if (!res.ok) {
        throw new Error(await res.text());
      }
      tunnels = await res.json();

      if (!quiet) {
        message = "Tunnels refreshed successfully";
        alertType = "success";
      }
    } catch (err) {
      message = err.message;
      alertType = "error";
    }
  }

  // Show ACME instruction
  async function getAcmeInstruction() {
    if (!domain) {
      message = "Please enter a domain first.";
      alertType = "error";
      return;
    }
    try {
      const res = await fetch(`/acme/${encodeURIComponent(domain)}`);
      if (!res.ok) {
        throw new Error(await res.text());
      }
      acmeOutput = await res.json();
      message = `ACME instructions fetched for ${domain}`;
      alertType = "success";
    } catch (err) {
      message = err.message;
      alertType = "error";
    }
  }

  // Validate ACME
  async function validateAcme() {
    if (!domain) {
      message = "Please enter a domain first.";
      alertType = "error";
      return;
    }
    try {
      const res = await fetch(`/validate/${encodeURIComponent(domain)}`);
      if (!res.ok) {
        throw new Error(await res.text());
      }
      acmeOutput = await res.json();
      message = `Validation request sent for ${domain}`;
      alertType = "success";
    } catch (err) {
      message = err.message;
      alertType = "error";
    }
  }

  // Unpublish tunnel
  function confirmUnpublish(hostname) {
    confirmMessage = `Are you sure you want to unpublish ${hostname}?`;
    actionToConfirm = async () => {
      try {
        const res = await fetch(`/unpublish/${encodeURIComponent(hostname)}`, {
          method: "POST",
        });
        if (!res.ok) {
          throw new Error(await res.text());
        }
        message = `Unpublished: ${hostname}`;
        alertType = "success";
        // Quietly refresh
        await loadTunnels(true);
      } catch (err) {
        message = err.message;
        alertType = "error";
      }
    };
    showConfirmModal = true;
  }

  // Release tunnel
  function confirmRelease(hostname) {
    confirmMessage = `Are you sure you want to release ${hostname}?`;
    actionToConfirm = async () => {
      try {
        const res = await fetch(`/release/${encodeURIComponent(hostname)}`, {
          method: "POST",
        });
        if (!res.ok) {
          throw new Error(await res.text());
        }
        message = `Released: ${hostname}`;
        alertType = "success";
        await loadTunnels(true);
      } catch (err) {
        message = err.message;
        alertType = "error";
      }
    };
    showConfirmModal = true;
  }

  // Confirm modal
  async function confirmAction() {
    if (actionToConfirm) {
      await actionToConfirm();
    }
    closeModal();
  }

  function closeModal() {
    showConfirmModal = false;
    confirmMessage = "";
    actionToConfirm = null;
  }

  // Synchronize with the server (previously called "reload")
  async function synchronize() {
    try {
      const res = await fetch("/reload", { method: "POST" });
      if (!res.ok) {
        throw new Error(await res.text());
      }
      message = "Synchronization successful!";
      alertType = "success";
      await loadTunnels(true);
    } catch (err) {
      message = err.message;
      alertType = "error";
    }
  }

  // Load current config
  async function loadConfig() {
    try {
      const res = await fetch("/config");
      if (!res.ok) {
        throw new Error(await res.text());
      }
      configText = await res.text();
      message = "Config loaded successfully";
      alertType = "success";
    } catch (err) {
      message = err.message;
      alertType = "error";
    }
  }

  // On mount, load tunnels quietly
  onMount(() => {
    loadTunnels(true);
  });
</script>

<!-- Container with fixed width to avoid shifting -->
<div class="flex flex-col min-h-screen">
  <div class="mx-auto w-[900px] px-4 py-6 flex-grow">
    <h1 class="text-3xl font-bold mb-4">Tunnel Manager</h1>

    <!-- Alert box (only shown if message != '') -->
    {#if message}
      <div
        class="mb-4 p-4 rounded border flex items-start justify-between"
        class:bg-red-50={alertType === "error"}
        class:bg-green-50={alertType === "success"}
        class:border-red-300={alertType === "error"}
        class:border-green-300={alertType === "success"}
        class:text-red-600={alertType === "error"}
        class:text-green-600={alertType === "success"}
      >
        <div class="pr-4">{message}</div>
        <button type="button" class="font-bold ml-4" on:click={closeAlert}>
          ✕
        </button>
      </div>
    {/if}

    <!-- Tabs Navigation -->
    <nav class="border-b border-gray-200 mb-6">
      <ul class="flex space-x-4">
        <li>
          <button
            class="py-2 px-3 text-gray-700 hover:text-blue-600 transition font-semibold"
            class:font-bold={activeTab === "tunnels"}
            on:click={() => (activeTab = "tunnels")}
          >
            Tunnels
          </button>
        </li>
        <li>
          <button
            class="py-2 px-3 text-gray-700 hover:text-blue-600 transition font-semibold"
            class:font-bold={activeTab === "acme"}
            on:click={() => (activeTab = "acme")}
          >
            ACME
          </button>
        </li>
        <li>
          <button
            class="py-2 px-3 text-gray-700 hover:text-blue-600 transition font-semibold"
            class:font-bold={activeTab === "publish"}
            on:click={() => (activeTab = "publish")}
          >
            Publish
          </button>
        </li>
        <li>
          <button
            class="py-2 px-3 text-gray-700 hover:text-blue-600 transition font-semibold"
            class:font-bold={activeTab === "config"}
            on:click={() => (activeTab = "config")}
          >
            Config
          </button>
        </li>
      </ul>
    </nav>

    <!-- Tab content area with min-height for consistent layout -->
    <div class="min-h-[600px] overflow-y-auto">
      {#if activeTab === "tunnels"}
        <!-- TUNNELS TAB -->
        <div class="space-y-6">
          <!-- General tunnel actions -->
          <div class="bg-white shadow rounded p-4">
            <h2 class="text-xl font-semibold mb-3">General Tunnel Actions</h2>
            <div class="flex items-center space-x-2">
              <button
                class="bg-blue-600 hover:bg-blue-700 text-white px-4 py-2 rounded transition"
                on:click={() => loadTunnels(false)}
              >
                Refresh Tunnels
              </button>
              <button
                class="bg-purple-600 hover:bg-purple-700 text-white px-4 py-2 rounded transition"
                on:click={synchronize}
              >
                Synchronize
              </button>
            </div>
          </div>

          <!-- Unpublish vs Release -->
          <div class="bg-white shadow rounded p-4">
            <h2 class="text-xl font-semibold mb-3">
              Difference between Unpublish and Release
            </h2>
            <ul class="list-disc list-inside text-gray-700">
              <li>
                <strong>Unpublish:</strong> Removes the hostname from being visible/active
                in the network. The application still “owns” it, so you can re-publish
                it later.
              </li>
              <li class="mt-1">
                <strong>Release:</strong> Unpublishes the hostname <em>and</em> relinquishes
                control. Once released, the hostname can be claimed by someone else.
              </li>
            </ul>
          </div>

          <!-- Registered Tunnels -->
          <div class="bg-white shadow rounded p-4">
            <h2 class="text-xl font-semibold mb-3">Registered Tunnels</h2>
            {#if tunnels.length === 0}
              <p class="text-gray-600">No tunnels found.</p>
            {:else}
              <div class="overflow-auto">
                <table class="table-auto w-full border-collapse">
                  <thead>
                    <tr class="bg-gray-100 border-b">
                      <th class="px-4 py-2 text-left">Hostname</th>
                      <th class="px-4 py-2 text-left">Target</th>
                      <th class="px-4 py-2 text-left">Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {#each tunnels as tunnel}
                      <tr class="border-b">
                        <td class="px-4 py-2 font-semibold"
                          >{tunnel.hostname}</td
                        >
                        <td class="px-4 py-2 text-gray-700"
                          >{tunnel.target}
                          {tunnel.headerHost
                            ? " (" + tunnel.headerHost + ")"
                            : ""}</td
                        >
                        <td class="px-4 py-2 space-x-2">
                          <button
                            class="bg-yellow-500 hover:bg-yellow-600 text-white px-3 py-1 rounded transition"
                            on:click={() => confirmUnpublish(tunnel.hostname)}
                          >
                            Unpublish
                          </button>
                          <button
                            class="bg-red-500 hover:bg-red-600 text-white px-3 py-1 rounded transition"
                            on:click={() => confirmRelease(tunnel.hostname)}
                          >
                            Release
                          </button>
                        </td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              </div>
            {/if}
          </div>
        </div>
      {:else if activeTab === "acme"}
        <!-- ACME TAB -->
        <div class="space-y-6">
          <div class="bg-white shadow rounded p-4">
            <h2 class="text-xl font-semibold mb-4">ACME Domain Management</h2>
            <label for="acmeDomain" class="block mb-2 font-medium"
              >Custom Domain</label
            >
            <input
              id="acmeDomain"
              class="border border-gray-300 p-2 w-full rounded focus:outline-none focus:border-blue-400"
              type="text"
              bind:value={domain}
              placeholder="e.g. custom.domain.com"
            />

            <div class="mt-4 flex items-center space-x-2">
              <button
                class="bg-green-600 hover:bg-green-700 text-white px-4 py-2 rounded transition"
                on:click={getAcmeInstruction}
              >
                Get ACME Instruction
              </button>
              <button
                class="bg-green-600 hover:bg-green-700 text-white px-4 py-2 rounded transition"
                on:click={validateAcme}
              >
                Validate ACME
              </button>
            </div>

            <!-- ACME Output -->
            {#if acmeOutput}
              <div class="mt-4 bg-gray-50 border border-gray-200 rounded p-4">
                <p class="font-semibold">{acmeOutput.message}</p>
                {#if acmeOutput.record}
                  <p class="mt-2">
                    <strong>Record:</strong>
                    {acmeOutput.record}
                  </p>
                {/if}
                {#if acmeOutput.type}
                  <p>
                    <strong>Type:</strong>
                    {acmeOutput.type}
                  </p>
                {/if}
                {#if acmeOutput.content}
                  <p>
                    <strong>Content:</strong>
                    {acmeOutput.content}
                  </p>
                {/if}
              </div>
            {/if}
          </div>
        </div>
      {:else if activeTab === "publish"}
        <!-- PUBLISH TAB -->
        <div class="space-y-6">
          <div class="bg-white shadow rounded p-4">
            <h2 class="text-xl font-semibold mb-4">How to Publish a Tunnel</h2>
            <p class="text-gray-700 mb-2">
              To publish a new tunnel, you must edit your config YAML and then
              synchronize the local config with the server. Here’s a simple
              example of a configuration:
            </p>
            <pre class="bg-gray-100 p-2 rounded text-sm overflow-x-auto mb-4">
version: 2
apex: apex.example.com:443
certificate: |
  -----BEGIN CERTIFICATE-----
  -----END CERTIFICATE-----
privKey: |
  -----BEGIN PRIVATE KEY-----
  -----END PRIVATE KEY-----</pre>

            <p class="text-gray-700 mb-2">
              To add a tunnel, insert a <code
                class="bg-gray-100 p-1 rounded text-sm">tunnels</code
              > stanza:
            </p>
            <pre class="bg-gray-100 p-2 rounded text-sm overflow-x-auto mb-4">
version: 2
apex: apex.example.com:443
certificate: |
  -----BEGIN CERTIFICATE-----
  -----END CERTIFICATE-----
privKey: |
  -----BEGIN PRIVATE KEY-----
  -----END PRIVATE KEY-----
tunnels:
  - target: tcp://127.0.0.1:1111</pre>

            <p class="text-gray-700 mb-2">
              If you have validated a custom domain via ACME, specify the
              hostname:
            </p>
            <pre class="bg-gray-100 p-2 rounded text-sm overflow-x-auto mb-4">
tunnels:
  - target: tcp://127.0.0.1:1111
    hostname: custom.domain.com</pre>

            <p class="text-gray-700">
              Otherwise, a random hostname will be generated automatically. Once
              you’ve updated the YAML, click <strong>Synchronize</strong> in the
              <strong>Tunnels</strong> tab to have the client apply the new configuration.
            </p>
          </div>
        </div>
      {:else if activeTab === "config"}
        <!-- CONFIG TAB -->
        <div class="space-y-6">
          <div class="bg-white shadow rounded p-4">
            <h2 class="text-xl font-semibold mb-4">Current Config</h2>

            <div class="mb-4 flex items-center space-x-2">
              <button
                class="bg-blue-600 hover:bg-blue-700 text-white px-4 py-2 rounded transition"
                on:click={loadConfig}
              >
                Refresh Config
              </button>
            </div>

            {#if configText}
              <pre
                class="bg-gray-100 p-2 rounded text-sm overflow-x-auto">{configText}</pre>
            {:else}
              <p class="text-gray-600">
                No config loaded. Click "Refresh Config" to load.
              </p>
            {/if}
          </div>
        </div>
      {/if}
    </div>
  </div>

  <!-- Footer -->
  <footer class="bg-gray-200 py-4 text-center text-sm text-gray-600">
    This UI was generated using ChatGPT
  </footer>

  <!-- Modal Overlay (only shown if showConfirmModal is true) -->
  {#if showConfirmModal}
    <div
      class="fixed inset-0 bg-gray-500 bg-opacity-75 transition-opacity z-40"
    ></div>

    <!-- Modal Content -->
    <div
      class="fixed inset-0 z-50 overflow-y-auto flex items-center justify-center p-4"
      aria-labelledby="modal-title"
      role="dialog"
      aria-modal="true"
    >
      <div
        class="bg-white rounded-lg shadow-xl transform transition-all sm:w-full sm:max-w-lg"
      >
        <div class="px-4 pt-5 pb-4 sm:p-6 sm:pb-4">
          <h3
            class="text-lg leading-6 font-medium text-gray-900 mb-4"
            id="modal-title"
          >
            Confirm Action
          </h3>
          <p class="text-gray-700">
            {confirmMessage}
          </p>
        </div>
        <div class="bg-gray-50 px-4 py-3 sm:px-6 sm:flex sm:flex-row-reverse">
          <button
            type="button"
            class="w-full inline-flex justify-center rounded-md border border-transparent shadow-sm
                   px-4 py-2 bg-red-600 text-base font-medium text-white hover:bg-red-700
                   sm:ml-3 sm:w-auto sm:text-sm transition"
            on:click={confirmAction}
          >
            Confirm
          </button>
          <button
            type="button"
            class="mt-3 w-full inline-flex justify-center rounded-md border border-gray-300 shadow-sm
                   px-4 py-2 bg-white text-base font-medium text-gray-700 hover:bg-gray-100
                   sm:mt-0 sm:ml-3 sm:w-auto sm:text-sm transition"
            on:click={closeModal}
          >
            Cancel
          </button>
        </div>
      </div>
    </div>
  {/if}
</div>
