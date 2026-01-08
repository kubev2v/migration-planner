export default class Alert {
    constructor(props = {}) {
        this.props = props;
    }

    render() {
        let self = this;
        switch (self.props.type) {
            case "error":
                return `
                <div class="pf-v6-c-alert pf-m-inline pf-m-danger">
                    <div class="pf-v6-c-alert__icon">
                        <svg class="pf-v6-svg" viewBox="0 0 512 512" fill="currentColor" aria-hidden="true" role="img" width="1em" height="1em"><path d="M504 256c0 136.997-111.043 248-248 248S8 392.997 8 256C8 119.083 119.043 8 256 8s248 111.083 248 248zm-248 50c-25.405 0-46 20.595-46 46s20.595 46 46 46 46-20.595 46-46-20.595-46-46-46zm-43.673-165.346l7.418 136c.347 6.364 5.609 11.346 11.982 11.346h48.546c6.373 0 11.635-4.982 11.982-11.346l7.418-136c.375-6.874-5.098-12.654-11.982-12.654h-63.383c-6.884 0-12.356 5.78-11.981 12.654z"></path>
                        </svg>
                    </div>
                    <h4 class="pf-v6-c-alert__title">
                        <span class="pf-v6-screen-reader">Danger alert:</span>
                            Error
                    </h4>
                    <div class="pf-v6-c-alert__description">
                        <ul class="pf-v6-c-list">
                            <li class="">Please double-check the URL is correct and reachable from within the VM.</li>
                        </ul>
                    </div>
                </div>
            `;
            case "invalidCredentials":
                return `
                <div class="pf-v6-c-alert pf-m-inline pf-m-danger">
                    <div class="pf-v6-c-alert__icon">
                        <svg class="pf-v6-svg" viewBox="0 0 512 512" fill="currentColor" aria-hidden="true" role="img" width="1em" height="1em">
                            <path d="M504 256c0 136.997-111.043 248-248 248S8 392.997 8 256C8 119.083 119.043 8 256 8s248 111.083 248 248zm-248 50c-25.405 0-46 20.595-46 46s20.595 46 46 46 46-20.595 46-46-20.595-46-46-46zm-43.673-165.346l7.418 136c.347 6.364 5.609 11.346 11.982 11.346h48.546c6.373 0 11.635-4.982 11.982-11.346l7.418-136c.375-6.874-5.098-12.654-11.982-12.654h-63.383c-6.884 0-12.356 5.78-11.981 12.654z">
                            </path>
                        </svg>
                    </div>
                    <h4 class="pf-v6-c-alert__title">
                        <span class="pf-v6-screen-reader">Danger alert:</span>
                            Invalid Credentials
                    </h4>
                    <div class="pf-v6-c-alert__description">
                        <ul class="pf-v6-c-list">
                            <li class="">Please double-check your entry for any typos.</li>
                            <li class="">Verify your account has not been temporarily locked for security reasons.</li>
                        </ul>
                    </div>
                </div>
            `
        }
    }
}
