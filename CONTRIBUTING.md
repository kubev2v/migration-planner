# Contributing to Migration Planner

Thank you for your interest in contributing to the Migration Planner project!  
We value your contributions and are excited to work with you to improve our codebase.

This guide will help you get started.

## How to contribute

This section focuses on contributing code to the Migration Planner project.

### Share Your Design

Before starting any implementation, please share your proposed design or approach for the feature or bug fix.  
This allows for early feedback and ensures alignment with the project's direction.  
To do this, please follow these steps:

1. **Open a Jira Ticket:** Ensure a Jira ticket exists for your change under the `ECOPROJECT` project with the `Assisted migration` component.
2. **Create a Design Document:** For new features, create a detailed design document and link it to the Jira ticket.
3. **Present Your Design:** Share your proposed change and design during the Assisted Migration Office Hours.

### Make Your Changes

* Implement your feature or bug fix.
* Ensure your code adheres to the project's coding style and conventions.
* Write clear, concise, and well-documented code.
* Add or update tests to cover your changes.
* Update documentation as needed (e.g., `README.md`, `docs/`).

### Coding Guidelines

To maintain consistency and quality, please adhere to the following coding guidelines:

* **Code Style:** Follow the established coding style of the existing codebase. Ensure your code passes the `make lint` checks.
* **Readability:** Write code that is easy to understand for other developers. Use meaningful variable and function names.
* **Modularity:** Break down complex problems into smaller, manageable functions or modules.
* **Error Handling:** Implement robust error handling for all potential failure points.
* **Comments:** Add comments where the code logic is not immediately obvious. Explain *why* a particular approach was taken, not just *what* the code does.
* **Performance:** Consider the performance implications of your code, especially for critical paths.
* **Security:** Be mindful of potential security vulnerabilities and follow best practices to prevent them.

### Create the Commit

Be sure to practice good git commit hygiene as you make your changes.  
All commits must be signed off, which can be done by adding the `-s` flag to your `git commit` command.  
**Note**: Before signing off your commits, please ensure your name and email are configured globally in Git:
```shell
git config --global user.name "Your Name"
git config --global user.email "your-email@redhat.com"
```
Use your git commits to provide context for the folks who will review PRs. We strive to follow [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/).

The commit message should follow this template:

```shell
[JIRA-TICKET] | [TYPE]: 

[optional BODY]

[optional FOOTER(s)]
```

For example:
```shell
ECOPROJECT-1234 | feat: add new network configuration options

The feature adds new settings to the network configuration to support IPv6.
The existing IPv4 logic remains unchanged.

Signed-off-by: Your Name <your-email@redhat.com>
```

The commit contains the following structural types, to communicate your intent:

* **fix:** A commit of the type `fix` patches a bug in your codebase (this correlates with `PATCH` in Semantic Versioning).
* **feat:** A commit of the type `feat` introduces a new feature to the codebase (this correlates with `MINOR` in Semantic Versioning).
* **build:** Changes that affect the build system or external dependencies
* **ci:** Changes to our CI configuration files and scripts
* **docs:** Documentation-only changes
* **perf:** A code change that improves performance
* **refactor:** A code change that neither fixes a bug nor adds a feature
* **style:** Changes that do not affect the meaning of the code (white-space, formatting, missing semi-colons, etc)
* **test:** Adding missing tests or correcting existing tests

### Test Your Changes

Comprehensive testing is crucial. Before submitting a pull request, ensure:

* **Validation Commands:** Before submitting your pull request, run the `make validate-all` command.  
This command is a collection of checks that ensure your code meets the project's standards.  
It includes:
  * `make lint` - Checks for code style and potential errors using a linter.
  * `make check-generate` - Verifies that auto-generated files are up to date.
  * `make check-format` - Ensures that the code formatting matches the project's standards.
  * `make unit-test` - Runs the project's unit tests to confirm code functionality, make sure existing unit tests should still pass.
* **Integration Tests:** If your changes involve interactions between different components, write or update integration tests to verify the end-to-end functionality.

### Create a Pull Request (PR)

1. Create a new pull request from your branch to the main branch.
2. Provide a clear and detailed description of your changes in the pull request description. Include:
   * What problem does this PR solve?
   * How was it solved?
   * Any relevant issue numbers (e.g., `Closes ECOPROJECT-XXX`, `Fixes ECOPROJECT-XXX`).
   * Screenshots or GIFs if your changes involve UI updates.
3. Request a review from one of the project maintainers.

### Additional Resources

* [**Contributing to a Project**](https://docs.github.com/en/get-started/exploring-projects-on-github/contributing-to-a-project): A general guide from GitHub on how to contribute to open-source projects.
* [**Git Basics**](https://git-scm.com/book/en/v2/Git-Basics-Getting-a-Git-Repository): A comprehensive resource for understanding fundamental Git commands and concepts.
* [**Conventional Commits**](https://www.conventionalcommits.org/en/v1.0.0/): The official specification for the structured commit messages we follow.
* [**DCO Sign Off**](https://cert-manager.io/docs/contributing/sign-off/): An explanation of the Developer Certificate of Origin (DCO), a legal statement required for all contributions, and why it is used.


## How to review

Thank you for taking the time to review contributions!  
Your feedback is crucial for maintaining the quality and stability of Migration Planner.

### General Review Guidelines

* **Be Constructive:** Provide clear, actionable, and polite feedback.
* **Focus on the Code:** Review for correctness, readability, maintainability, and adherence to project standards.
* **Test the Changes:** If possible, pull the branch locally and test the changes to verify the reported behavior.
* **Check Documentation:** Ensure that any new features or changes are adequately documented.
* **Consider Edge Cases:** Think about how the changes might affect different scenarios or edge cases.
* **Approve or Request Changes:** Once you are satisfied with the PR, approve it. If there are issues, request changes and explain why.

### Checklist for Reviewers

* \[ \] **Code Correctness:** Does the code work as intended? Does it solve the problem described in the PR?
* \[ \] **Readability:** Is the code easy to understand? Are variable and function names clear?
* \[ \] **Maintainability:** Is the code well-structured? Is it easy to extend or modify in the future?
* \[ \] **Tests:** Are there sufficient tests to cover the changes? Do the tests pass?
* \[ \] **Documentation:** Is the documentation updated if necessary (e.g., `README.md`, inline comments)?
* \[ \] **Performance/Security (if applicable):** Are there any performance bottlenecks or security vulnerabilities introduced?
* \[ \] **Style:** Does the code adhere to the project's coding style guidelines?
* \[ \] **Commit Message:** Is the commit message clear and descriptive?
* \[ \] **PR Description:** Is the PR description clear and comprehensive?
