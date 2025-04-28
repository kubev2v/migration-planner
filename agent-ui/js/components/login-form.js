import FormStates from "../commons/states.js";
import Alert from "./alert.js";

export default class LoginForm {
    constructor(props = {}) {
        this.props = props;
    }

    helperText(msg) {
        return `
            <div class="pf-v6-c-form__helper-text">
                <div class="pf-v6-c-helper-text">
                    <div class="pf-v6-c-helper-text__item pf-m-error">
                        <span class="pf-v6-c-helper-text__item-icon">
                            <svg class="pf-v6-svg" viewBox="0 0 512 512" fill="currentColor" aria-hidden="true" role="img" width="1em" height="1em">
                                <path d="M504 256c0 136.997-111.043 248-248 248S8 392.997 8 256C8 119.083 119.043 8 256 8s248 111.083 248 248zm-248 50c-25.405 0-46 20.595-46 46s20.595 46 46 46 46-20.595 46-46-20.595-46-46-46zm-43.673-165.346l7.418 136c.347 6.364 5.609 11.346 11.982 11.346h48.546c6.373 0 11.635-4.982 11.982-11.346l7.418-136c.375-6.874-5.098-12.654-11.982-12.654h-63.383c-6.884 0-12.356 5.78-11.981 12.654z"></path>
                            </svg>
                        </span>
                        <span class="pf-v6-c-helper-text__item-text">${msg}</span>
                    </div>
                </div>
            </div>
        `
    }

    render() {
        let self = this;
        let formElement = document.createElement('form');
        formElement.className = "pf-v6-c-form";
        formElement.id = "login-form";

        const disabled = self.props.disabled ? { elem: "disabled", class: "pf-m-disabled" } : { elem: "", class: "" };

        let alert = null;
        let urlInfoElement = new InfoElement({ state: "" });
        let credsInfoElement = new InfoElement({ state: "" });

        if (!self.props.disabled) {
            switch (self.props.state) {
                case FormStates.CredentialsRejected:
                    alert = new Alert({ type: "invalidCredentials" });
                    credsInfoElement = new InfoElement({ state: "error" });
                    urlInfoElement = new InfoElement({ state: "success" });
                    break;
                case FormStates.InvalidCredentials:
                    alert = new Alert({ type: "error" });
                    urlInfoElement = new InfoElement({ state: "error" });
                    break;
            }
        }

        formElement.innerHTML = `
                    <div class="pf-v6-c-form__group">
                        <div class="pf-v6-c-form__group-label">
                            <label class="pf-v6-c-form__label" for="url-form-control">
                                <span class="pf-v6-c-form__label-text">Environment URL</span>
                                <span class="pf-v6-c-form__label-required" aria-hidden="true">*</span>
                            </label>
                        </div>
                        <div class="pf-v6-c-form__group-control">
                            <span class="pf-v6-c-form-control ${disabled.class} ${urlInfoElement.class()}">
                                <input id="url-form-control" name="url" pattern="https://.*" type="url" required=""
                                    ${disabled.elem}
                                    placeholder="https://vcenter_server_ip_address_or_fqdn"
                                    value=${self.props.credentials.url ? self.props.credentials.url : ""}
                                >
                                ${urlInfoElement.render()}
                            </span>
                            ${!self.props.valid && self.props.invalidElement == "url" ? self.helperText(self.props.urlHelperText) : ""}
                        </div>
                    </div>
                    <div class="pf-v6-c-form__group">
                        <div class="pf-v6-c-form__group-label">
                            <label class="pf-v6-c-form__label" for="username-form-control">
                                <span class="pf-v6-c-form__label-text">VMware Username</span>
                                <span class="pf-v6-c-form__label-required">*</span>
                            </label>
                        </div>
                        <div class="pf-v6-c-form__group-control">
                            <span class="pf-v6-c-form-control ${disabled.class} ${credsInfoElement.class()}">
                                <input id="username-form-control" name="username"
                                    aria-describedby="username-helper-text" type="email" required=""
                                    ${disabled.elem}
                                    placeholder="su.do@redhat.com" 
                                    value=${self.props.credentials.username ? self.props.credentials.username : ""}
                                >
                                ${credsInfoElement.render()}
                            </span>
                            ${!self.props.valid && self.props.invalidElement == 'username' ? self.helperText(self.props.usernameHelperText) : ""
            }
                        </div>
                    </div >
                    <div class="pf-v6-c-form__group">
                        <div class="pf-v6-c-form__group-label">
                            <label class="pf-v6-c-form__label" for="password-form-control">
                                <span class="pf-v6-c-form__label-text">Password</span>
                                <span class="pf-v6-c-form__label-required"> *</span>
                            </label>
                        </div>
                        <div class="pf-v6-c-form__group-control">
                            <span class="pf-v6-c-form-control ${disabled.class} ${credsInfoElement.class()}">
                                <input id="password-form-control" name="password"
                                    ${disabled.elem}
                                    aria-describedby="password-helper-text" type="password" required=""
                                    value=${self.props.credentials.password ? self.props.credentials.password : ""}>
                                ${credsInfoElement.render()}
                            </span>
                        </div>
                    </div>
                    <div class="pf-v6-c-form__group">
                        <div class="pf-v6-c-form__group-control">
                            <div class="pf-v6-c-check ${disabled.elem}">
                                <input id="checkbox-form-control" class="pf-v6-c-check__input"
                                    name="isDataSharingAllowed" type="checkbox" aria-invalid="false" data"
                                    ${disabled.elem}
                                    ${self.props.dataSharingEnabled ? "checked" : ""}>
                                <label class="pf-v6-c-check__label" for="checkbox-form-control">
                                    I agree to
                                    share aggregated data about my environment with
                                    Red Hat.
                                </label>
                            </div>
                        </div>
                    </div>
`;

        if (alert) {
            formElement.innerHTML = `
                ${formElement.innerHTML}
                ${alert.render()}
`;
        }


        let dataSharingElement = formElement.querySelector("#checkbox-form-control");
        dataSharingElement.addEventListener("click", () => {
            self.props.enableDataSharingCallback(dataSharingElement);
        });

        return formElement;
    }
}

class InfoElement {
    constructor(props = {}) {
        this.props = props;
    }

    class() {
        let self = this;
        switch (self.props.state) {
            case "error":
                return "pf-m-error";
            case "success":
                return "pf-m-success";
            default:
                return ""
        }
    }


    render() {
        let self = this;
        switch (self.props.state) {
            case "error":
                return `
    <span class="pf-v6-c-form-control__utilities">
        <span class="pf-v6-c-form-control__icon pf-m-status">
            <svg class="pf-v6-svg" viewBox="0 0 512 512" fill="currentColor" aria-hidden="true" role="img" width="1em" height="1em">
                <path d="M504 256c0 136.997-111.043 248-248 248S8 392.997 8 256C8 119.083 119.043 8 256 8s248 111.083 248 248zm-248 50c-25.405 0-46 20.595-46 46s20.595 46 46 46 46-20.595 46-46-20.595-46-46-46zm-43.673-165.346l7.418 136c.347 6.364 5.609 11.346 11.982 11.346h48.546c6.373 0 11.635-4.982 11.982-11.346l7.418-136c.375-6.874-5.098-12.654-11.982-12.654h-63.383c-6.884 0-12.356 5.78-11.981 12.654z"></path>
            </svg>
        </span>
                    </span >
    `
            case "success":
                return `
    <span class="pf-v6-c-form-control__utilities">
        <span class="pf-v6-c-form-control__icon pf-m-status">
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" fill="currentColor" aria-hidden="true" role="img" width="1em" height="1em">
                <path d="M256 512A256 256 0 1 0 256 0a256 256 0 1 0 0 512zM369 209L241 337c-9.4 9.4-24.6 9.4-33.9 0l-64-64c-9.4-9.4-9.4-24.6 0-33.9s24.6-9.4 33.9 0l47 47L335 175c9.4-9.4 24.6-9.4 33.9 0s9.4 24.6 0 33.9z" /></svg>
        </span>
                    </span >
    `
            default:
                return ""
        }
    }
}
