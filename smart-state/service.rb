require 'webrick'
require 'json'
require 'manageiq-smartstate'
# boot.rb
$LOAD_PATH << File.expand_path("/home/bodnopoz/work/manageiq-gems-pending/lib", __FILE__)
$LOAD_PATH << File.expand_path("/home/bodnopoz/work/manageiq-gems-pending/lib/gems/pending", __FILE__)
require 'manageiq/gems/pending'
require 'MiqVm/MiqVm'
require 'ostruct'
require 'VMwareWebService/MiqVim'
require "active_support"
require "active_support/core_ext/array"
require 'logger'
require 'timeout'

$vim_log = $log = Logger.new(STDERR)

port = ENV['SMART_STATE_SERVICE_PORT'] || 3334
timeout_sec = ENV['SMART_STATE_SERVICE_TIMEOUT_SEC'] || 20

results_file_path = '/tmp/smart-scan-results.json'

# Create the HTTP server
server = WEBrick::HTTPServer.new(Port: port.to_i)

server.mount_proc '/results' do |req, res|
  if req.request_method == 'GET'
    begin
      if File.exist?(results_file_path)
        # File exists, read and return its contents as JSON
        file_contents = File.read(results_file_path)
        res.status = 200
        res['Content-Type'] = 'application/json'
        res.body = file_contents
      else
        res.status = 202
        res['Content-Type'] = 'application/json'
        res.body = {
          status: 'unknown',
          message: 'No results available yet. The process may not have started or is still in progress.'
        }.to_json
      end
    rescue StandardError => e
      # Handle any unexpected errors
      res.status = 500
      res['Content-Type'] = 'application/json'
      res.body = {
        status: 'error',
        message: 'An error occurred while processing the request.',
        details: e.message
      }.to_json
    end
  else
    # Handle unsupported methods
    res.status = 405
    res['Content-Type'] = 'application/json'
    res.body = { status: 'error', message: 'Method not allowed' }.to_json
  end
end
# Define an endpoint to handle requests
server.mount_proc '/init_scan' do |req, res|
  if req.request_method == 'POST'
    begin
      begin
        body = JSON.parse(req.body)
        host_address = body['server']
        username = body['username']
        password = body['password']
      rescue JSON::ParserError => e
        res.status = 400
        res['Content-Type'] = 'application/json'
        res.body = { status: 'error', message: 'Invalid JSON payload', details: e.message }.to_json
        return
      end

      # Respond immediately to the client
      res.status = 200
      res['Content-Type'] = 'application/json'
      res.body = { status: 'success', message: 'Scan initiated.' }.to_json

      # Start the heavy lifting in a separate thread
      Thread.new do
        begin
          puts 'starting scan thread'

          vim = MiqVim.new(server: host_address, username: username, password: password)

          all_vms_extracted_data = []
          CATEGORIES = %w(accounts services software system vmconfig)


          vm_count = 0
          num_of_vms = vim.virtualMachines.keys.size
          start_time = Time.now

          vim.virtualMachines.each do |vm_name, vm_props|
            begin
              max_vms = max_vms_to_scan
              num_vms_to_scan = max_vms ? max_vms.to_i : num_of_vms
              break if max_vms && vm_count >= max_vms
              puts "now scanning key: #{vm_name} which is #{vm_count} out of #{num_vms_to_scan}"
              ds_path = vm_props.dig('summary', 'config', 'vmPathName')
              next if ds_path.to_s.strip.empty?

              ost = OpenStruct.new
              ost.miqVim = vim
              vm = MiqVm.new(ds_path, ost)

              vm_data_by_category = {}
              CATEGORIES.each do |cat|
                begin
                  Timeout.timeout(timeout_sec) do
                    xml = vm.extract(cat)
                    vm_data_by_category[cat] = xml.to_s
                  end
                rescue Timeout::Error
                  # Handle the timeout error
                  puts "Timed out while extracting #{cat} for VM '#{vm_name}'"
                  vm_data_by_category[cat] = { 'error' => 'Timeout: Extraction took too long' }
                rescue => err
                  vm_data_by_category[cat] = { 'error' => err.message }
                end
              end

              all_vms_extracted_data << {
                'vm_name' => vm_name,
                'datastore_path' => ds_path,
                'categories' => vm_data_by_category
              }
            rescue => e
              all_vms_extracted_data << {
                'vm_name' => vm_name,
                'datastore_path' => ds_path,
                'error' => e.message
              }
            end
            vm_count += 1
          end

          end_time = Time.now
          elapsed_time = end_time - start_time
          puts "Processing all VMs took #{elapsed_time.round(2)} seconds."

          # Save the results to a file
          File.open(results_file_path, 'w') do |file|
            file.write(JSON.pretty_generate(all_vms_extracted_data))
          end
        rescue => e
          $log.error("Error in background thread: #{e.message}")
        ensure
          vim.disconnect if vim
        end
      end

    rescue StandardError => e
      # Handle errors
      res.status = 500
      res['Content-Type'] = 'application/json'
      res.body = { status: 'error', message: e.message }.to_json
    end
  else
    # Handle unsupported methods
    res.status = 405
    res['Content-Type'] = 'application/json'
    res.body = { status: 'error', message: 'Method not allowed' }.to_json
  end
end

def max_vms_to_scan
  File.exist?('max_vms_to_scan.txt') ? File.read('max_vms_to_scan.txt').to_i : ENV['MAX_VMS_TO_SCAN']&.to_i
end

# Graceful shutdown on CTRL+C
trap 'INT' do
  server.shutdown
end

# Start the server
server.start
