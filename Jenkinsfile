pipeline{
    agent {
        label 'container-agent'
    }
    tools {
        go 'go1.19.2'
    }
    stages{
        stage("Short Tests with Race Detector"){
            steps{
                sh "go test -v -short -race -timeout 120s ./..."
            }
        }
        stage("Run Integration Tests"){
            environment{
                GO_RUN_INTEGRATION = '1'
            }
            steps{
                sh "make certs"
                sh "go test -v -timeout 60s ./integrations"
            }
        }
        stage("Run Full Test Suite"){
            steps{
                sh "make full_test TIMEOUT=600s"
            }
        }
    }
}