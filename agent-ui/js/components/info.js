export default class InfoDiscovery {
    constructor() {
    }

    render() {
        let infoElement = document.createElement('div');
        infoElement.className = "pf-v5-c-card__body";

        infoElement.innerHTML = `
                <div class="alert-ok">
                <div class="pf-v6-c-alert pf-m-inline pf-m-success">
                    <div class="pf-v6-c-alert__icon">
                        <svg class="pf-v6-svg" viewBox="0 0 512 512" fill="currentColor" aria-hidden="true" role="img" width="1em" height="1em">
                            <path d="M504 256c0 136.967-111.033 248-248 248S8 392.967 8 256 119.033 8 256 8s248 111.033 248 248zM227.314 387.314l184-184c6.248-6.248 6.248-16.379 0-22.627l-22.627-22.627c-6.248-6.249-16.379-6.249-22.628 0L216 308.118l-70.059-70.059c-6.248-6.248-16.379-6.248-22.628 0l-22.627 22.627c-6.248 6.248-6.248 16.379 0 22.627l104 104c6.249 6.249 16.379 6.249 22.628.001z">
                            </path>
                        </svg>
                    </div>
                    <h4 class="pf-v6-c-alert__title">
                        <span class="pf-v6-screen-reader">Success alert:</span>
                            Connected
                    </h4>
                    <div class="pf-v6-c-alert__description">
                        <ul class="pf-v6-c-list">
                            <li class="">The migration discovery VM is connected to your VMware environment</li>
                        </ul>
                    </div>
                </div>
                    <p>
                        <span class="pf-v6-c-icon pf-m-inline pf-m-2xl">
                            <span class="pf-v6-c-icon__content">
                                <svg class="pf-v6-svg" viewBox="0 0 512 512" fill="currentColor" aria-hidden="true" role="img" color="#3e8635">
                                    <path d="M504 256c0 136.967-111.033 248-248 248S8 392.967 8 256 119.033 8 256 8s248 111.033 248 248zM227.314 387.314l184-184c6.248-6.248 6.248-16.379 0-22.627l-22.627-22.627c-6.248-6.249-16.379-6.249-22.628 0L216 308.118l-70.059-70.059c-6.248-6.248-16.379-6.248-22.628 0l-22.627 22.627c-6.248 6.248-6.248 16.379 0 22.627l104 104c6.249 6.249 16.379 6.249 22.628.001z">
                                    </path>
                                </svg>
                            </span>
                        </span>
                        <span>Discovery completed</span>
                    </p>
                </div>
                `
        return infoElement;
    }
}
