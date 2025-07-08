export default class UpdateStatus {
    constructor(props = {}) {
        this.props = props;
    }

    render() {
        const renderInfoElement = (hasError, message) => {
            let e = document.createElement("div");
            if (hasError) {
                e.innerHTML = `
                <svg class="pf-v6-svg" viewBox="0 0 512 512" fill="currentColor" aria-hidden="true" role="img" width="1em" height="1em" color="var(--pf-t--global--icon--color--status--danger--default)"><path d="M504 256c0 136.997-111.043 248-248 248S8 392.997 8 256C8 119.083 119.043 8 256 8s248 111.083 248 248zm-248 50c-25.405 0-46 20.595-46 46s20.595 46 46 46 46-20.595 46-46-20.595-46-46-46zm-43.673-165.346l7.418 136c.347 6.364 5.609 11.346 11.982 11.346h48.546c6.373 0 11.635-4.982 11.982-11.346l7.418-136c.375-6.874-5.098-12.654-11.982-12.654h-63.383c-6.884 0-12.356 5.78-11.981 12.654z"></path></svg>
                <p>${message}</p>
                `;
                return e;
            }
            e.innerHTML = `
            <svg class="pf-v6-svg" viewBox="0 0 512 512" fill="currentColor" aria-hidden="true" role="img" width="1em" height="1em" color="var(--pf-t--global--icon--color--status--success--default)"><path d="M504 256c0 136.967-111.033 248-248 248S8 392.967 8 256 119.033 8 256 8s248 111.033 248 248zM227.314 387.314l184-184c6.248-6.248 6.248-16.379 0-22.627l-22.627-22.627c-6.248-6.249-16.379-6.249-22.628 0L216 308.118l-70.059-70.059c-6.248-6.248-16.379-6.248-22.628 0l-22.627 22.627c-6.248 6.248-6.248 16.379 0 22.627l104 104c6.249 6.249 16.379 6.249 22.628.001z"></path></svg>
            <p>${message}</p>
            `;
            return e;
        };

        let self = this;
        let statusElement = document.createElement("div");
        statusElement.classList = "";

        let hasError = false;
        let message = self.props.successMessage;
        if (self.props.updateSuccessful == "false") {
            hasError = true;
            message = self.props.message;
        }
        statusElement.appendChild(renderInfoElement(hasError, message));
        return statusElement;
    }
}
